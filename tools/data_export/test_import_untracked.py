"""Check preservation and compatibility boundaries of local asset restoration."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

import regenerate

from import_untracked import restore, EXTERNAL_ASSET_DIRECTORIES, EXTERNAL_IMPORT_FORMAT


class ImportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / 'repo'
        self.source = Path(self.temp.name) / 'generated'
        self.repo.mkdir()
        self.source.mkdir()
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        (self.repo / '.gitignore').write_text('/data/**/*.png\n')
        (self.repo / 'data').mkdir()
        doc = {'source': '/old/Wonderland-Client/aLogin.exe', 'source_sha256': 'verified-build', 'sprites': [{'frame': 'sprite.png'}]}
        (self.repo / 'data' / 'editable.json').write_text(json.dumps(doc))
        subprocess.run(['git', 'add', '.gitignore', 'data/editable.json'], cwd=self.repo, check=True)
        doc['source'] = '/new/Wonderland-Client/aLogin.exe'
        (self.source / 'editable.json').write_text(json.dumps(doc))
        (self.source / 'sprite.png').write_bytes(b'original-pixels')
        (self.source / 'untracked.json').write_text('{}')

    def test_restore_only_missing_ignored_files_and_preserve_edits(self):
        metadata = (self.repo / 'data' / 'editable.json').read_bytes()
        self.assertEqual(restore(self.source, self.repo, dry_run=True), 1)
        self.assertFalse((self.repo / 'data' / 'sprite.png').exists())
        self.assertEqual(restore(self.source, self.repo), 1)
        self.assertEqual((self.repo / 'data' / 'sprite.png').read_bytes(), b'original-pixels')
        self.assertEqual((self.repo / 'data' / 'editable.json').read_bytes(), metadata)
        self.assertFalse((self.repo / 'data' / 'untracked.json').exists())
        (self.repo / 'data' / 'sprite.png').write_bytes(b'edited-pixels')
        self.assertEqual(restore(self.source, self.repo), 0)
        self.assertEqual((self.repo / 'data' / 'sprite.png').read_bytes(), b'edited-pixels')

    def test_retired_database_exports_are_not_restored_from_old_output(self):
        with (self.repo / '.gitignore').open('a') as ignored:
            ignored.write('/data/server/database_data.json\n/data/server/database.override.json\n')
        (self.source / 'server').mkdir()
        (self.source / 'server' / 'database_data.json').write_text('{"private": "account data"}')
        (self.source / 'server' / 'database.override.json').write_text('{}')
        self.assertEqual(restore(self.source, self.repo), 1)
        self.assertFalse((self.repo / 'data' / 'server' / 'database_data.json').exists())
        self.assertFalse((self.repo / 'data' / 'server' / 'database.override.json').exists())

    def test_external_restore_needs_no_server_or_root_gameplay_exports(self):
        for directory in EXTERNAL_ASSET_DIRECTORIES:
            (self.source / directory).mkdir()
        scope = {'schema_version': 1, 'format': EXTERNAL_IMPORT_FORMAT,
                 'directories': list(EXTERNAL_ASSET_DIRECTORIES)}
        (self.source / 'external_asset_import.json').write_text(json.dumps(scope))
        (self.repo / 'data' / 'server').mkdir()
        (self.repo / 'data' / 'server' / 'starter_items.json').write_text('[42]')
        (self.repo / 'data' / 'media').mkdir()
        metadata = '{"source_sha256":"same-build","image":"sprite.png"}'
        (self.repo / 'data' / 'media' / 'editable.json').write_text(metadata)
        (self.source / 'media' / 'editable.json').write_text(metadata)
        (self.source / 'media' / 'sprite.png').write_bytes(b'pixels')
        subprocess.run(['git', 'add', 'data/server', 'data/media/editable.json'], cwd=self.repo, check=True)
        self.assertEqual(restore(self.source, self.repo), 1)
        self.assertEqual((self.repo / 'data' / 'media' / 'sprite.png').read_bytes(), b'pixels')
        self.assertEqual((self.repo / 'data' / 'server' / 'starter_items.json').read_text(), '[42]')
        self.assertFalse((self.repo / 'data' / 'sprite.png').exists())

    def test_reject_different_build_before_copying(self):
        document = json.loads((self.source / 'editable.json').read_text())
        document['source_sha256'] = 'different-build'
        (self.source / 'editable.json').write_text(json.dumps(document))
        with self.assertRaisesRegex(ValueError, 'metadata differs'):
            restore(self.source, self.repo)
        self.assertFalse((self.repo / 'data' / 'sprite.png').exists())

    def test_reject_destination_link_outside_data(self):
        elsewhere = Path(self.temp.name) / 'elsewhere'
        elsewhere.mkdir()
        (self.source / 'escape').mkdir()
        (self.source / 'escape' / 'sprite.png').write_bytes(b'pixels')
        (self.repo / 'data' / 'escape').symlink_to(elsewhere, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'escapes data'):
            restore(self.source, self.repo)
        self.assertFalse((elsewhere / 'sprite.png').exists())


class RegenerationTests(unittest.TestCase):
    def test_supplied_roots_reach_every_exporter(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            client = root / 'WLRI with spaces'
            output = root / 'generated'
            for directory in (client / 'data', client / 'jma', client / 'pic'):
                directory.mkdir(parents=True)
            (client / 'aLogin.exe').write_bytes(b'matching executable')
            with mock.patch.object(regenerate, 'extract') as audio, mock.patch.object(regenerate, 'export_media') as media, mock.patch.object(regenerate.subprocess, 'run') as commands:
                regenerate.regenerate(output, client)
            audio.assert_called_once_with(client / 'data', output / 'audio')
            scope = json.loads((output / 'external_asset_import.json').read_text())
            self.assertEqual(scope['format'], EXTERNAL_IMPORT_FORMAT)
            self.assertEqual(scope['directories'], list(EXTERNAL_ASSET_DIRECTORIES))
            media.assert_called_once_with(client, output / 'media')
            sprite = commands.call_args_list[0].args[0]
            picture = commands.call_args_list[1].args[0]
            self.assertIn(str(client / 'jma'), sprite)
            self.assertIn(str(client / 'aLogin.exe'), sprite)
            self.assertIn(str(client / 'pic'), picture)


if __name__ == '__main__':
    unittest.main()

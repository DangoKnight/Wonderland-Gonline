"""Independent source selection and bilingual identity matching tests."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from export import sha256, write_json
from text_languages import annotate_languages, verify_language_sources
import regenerate


class LanguageSourcesTests(unittest.TestCase):
    def test_bilingual_text_preserves_primary_records_and_manifest(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / 'Talk.dat'
            source.write_bytes(b'original companion table')
            # Talk identity includes the first two ID bytes; text occupies the
            # following fixed field. A mismatched ID must not supply text.
            row = {'fields': {'id': 17}, 'text': {'text': 'English'},
                   'decoded_hex': (b'\x11\x00' + bytes(290)).hex()}
            output = root / 'talk_data.json'
            write_json(output, {'source_sha256': 'native-hash', 'records': [row]})
            write_json(root / 'asset_manifest.json', {'assets': [{'output': output.name}]})
            chinese = dict(row, text={'text': '\u4e2d\u6587'})
            wrong = dict(row, fields={'id': 18}, text={'text': 'wrong'}, decoded_hex=(b'\x12\x00' + bytes(290)).hex())
            with mock.patch('text_languages.native_rows', return_value=(source, [chinese, wrong])):
                annotate_languages(root, 'en', root)
            doc = json.loads(output.read_text(encoding='utf-8'))
            self.assertEqual(doc['records'], [row])
            self.assertEqual(doc['source_sha256'], 'native-hash')
            self.assertEqual(doc['text_languages'], {'zh-TW': {'talk.17.text': '\u4e2d\u6587'}})
            self.assertEqual(doc['text_language_sources']['zh-TW']['source_sha256'], sha256(source))
            entry = json.loads((root / 'asset_manifest.json').read_text(encoding='utf-8'))['assets'][0]
            self.assertEqual(entry['output_sha256'], sha256(output))
            self.assertEqual(entry['output_bytes'], output.stat().st_size)
            verify_language_sources(doc)
            source.write_bytes(b'changed companion')
            with self.assertRaisesRegex(ValueError, 'companion text source changed'):
                verify_language_sources(doc)

    def test_single_source_regeneration_uses_only_that_installation(self):
        for language in ('en', 'zh-TW'):
            with self.subTest(language=language), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                client, output = root / 'only installation', root / 'output'
                for folder in ('data', 'jma', 'pic', 'menu/Skins/default'):
                    (client / folder).mkdir(parents=True)
                (client / 'aLogin.exe').write_bytes(b'executable')
                (client / 'menu/Skins/default/Button.bmp').write_bytes(b'image')
                with mock.patch.object(regenerate, 'export_all'), mock.patch.object(regenerate, 'preserve_server_exports'), mock.patch.object(regenerate, 'annotate_languages') as texts, mock.patch.object(regenerate, 'export_media') as media, mock.patch.object(regenerate.subprocess, 'run'):
                    regenerate.regenerate(output, client, client_language=language)
                texts.assert_called_once_with(output, language, None)
                if language == 'en':
                    media.assert_called_once_with(client, output / 'media')
                else:
                    media.assert_called_once_with(client, output / 'media', ui_language='zh-TW', chinese_client=None)

    def test_invalid_companion_fails_before_any_extraction(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            client, companion, output = root / 'en', root / 'zh', root / 'output'
            client.mkdir()
            (companion / 'data').mkdir(parents=True)
            with self.assertRaisesRegex(ValueError, 'missing language source'):
                regenerate.regenerate(output, client, chinese_client=companion)
            self.assertFalse(output.exists())

    def test_both_sources_default_to_both_ui_sets(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            client, chinese, output = root / 'en', root / 'zh', root / 'output'
            for folder in ('data', 'jma', 'pic'):
                (client / folder).mkdir(parents=True)
            (client / 'aLogin.exe').write_bytes(b'executable')
            (chinese / 'menu/Skins/default').mkdir(parents=True)
            (chinese / 'menu/Skins/default/Button.bmp').write_bytes(b'image')
            with mock.patch.object(regenerate, 'native_rows'), mock.patch.object(regenerate, 'export_all'), mock.patch.object(regenerate, 'preserve_server_exports'), mock.patch.object(regenerate, 'annotate_languages') as texts, mock.patch.object(regenerate, 'export_media') as media, mock.patch.object(regenerate.subprocess, 'run'):
                regenerate.regenerate(output, client, chinese_client=chinese)
            texts.assert_called_once_with(output, 'en', chinese)
            media.assert_called_once_with(client, output / 'media', ui_language='both', chinese_client=chinese)


if __name__ == '__main__':
    unittest.main()

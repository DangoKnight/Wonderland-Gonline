"""UI image selection, provenance and single-language extraction fixtures."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from PIL import Image
from media import export_media
from ui import export_ui, verify_ui
from verify_media import verify


class UITests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.english, self.chinese = self.root / 'english', self.root / 'chinese'
        for client, color in [(self.english, (200, 0, 0)), (self.chinese, (0, 200, 0))]:
            directory = client / 'menu' / 'Skins' / 'white'
            directory.mkdir(parents=True)
            Image.new('RGB', (3, 4), color).save(directory / 'Button.bmp')
            Image.new('RGB', (3, 4), color).save(directory / 'Background.jpg')

    def test_each_ui_set_can_be_imported_alone_and_preserves_originals(self):
        for code, client in [('en', self.english), ('zh-TW', self.chinese)]:
            original = {p: p.read_bytes() for p in client.rglob('*') if p.is_file()}
            output = self.root / code
            doc = export_ui(client, output, code)
            self.assertEqual(len(doc['files']), 2)
            self.assertEqual(sorted(p.name for p in (output / 'locales').iterdir()), [code])
            self.assertTrue((output / 'locales' / code / 'menu/Skins/white/Background.jpg.png').is_file())
            self.assertEqual(verify_ui(output / 'locales' / code / 'skin_manifest.json')['language'], code)
            self.assertEqual(original, {p: p.read_bytes() for p in original})
            with self.assertRaisesRegex(ValueError, 'already exists'):
                export_ui(client, output, code)

    def test_complete_media_selection_and_verification(self):
        for selection in ('en', 'zh-TW', 'both'):
            output = self.root / ('media-' + selection)
            with mock.patch('media.shutil.which', return_value='ffmpeg'):
                doc = export_media(self.english, output, selection, self.chinese)
            has_en = (output / 'menu/Skins/white/Button.bmp.png').is_file()
            has_zh = (output / 'locales/zh-TW/menu/Skins/white/Button.bmp.png').is_file()
            self.assertEqual(has_en, selection != 'zh-TW')
            self.assertEqual(has_zh, selection != 'en')
            self.assertEqual(doc['ui_languages'], ['en', 'zh-TW'] if selection == 'both' else [selection])
            verify(output)

    def test_invalid_both_source_fails_before_creating_output(self):
        output = self.root / 'bad'
        with mock.patch('media.shutil.which', return_value='ffmpeg'):
            with self.assertRaisesRegex(ValueError, 'chinese-client'):
                export_media(self.english, output, 'both')
        self.assertFalse(output.exists())

    def test_media_output_cannot_modify_chinese_source(self):
        output = self.chinese / 'generated'
        with mock.patch('media.shutil.which', return_value='ffmpeg'):
            with self.assertRaisesRegex(ValueError, 'outside Chinese UI'):
                export_media(self.english, output, 'both', self.chinese)
        self.assertFalse(output.exists())

    def test_modified_ui_output_is_detected(self):
        output = self.root / 'tampered'
        export_ui(self.chinese, output, 'zh-TW')
        (output / 'locales/zh-TW/menu/Skins/white/Button.bmp.png').write_bytes(b'changed')
        with self.assertRaisesRegex(ValueError, 'UI output changed'):
            verify_ui(output / 'locales/zh-TW/skin_manifest.json')

    def test_adding_ui_to_existing_media_preserves_verifiable_manifest(self):
        output = self.root / 'extended'
        with mock.patch('media.shutil.which', return_value='ffmpeg'):
            export_media(self.english, output)
        export_ui(self.chinese, output, 'zh-TW')
        verify(output)
        manifest = json.loads((output / 'media_manifest.json').read_text(encoding='utf-8'))
        self.assertEqual(manifest['ui_languages'], ['en', 'zh-TW'])


if __name__ == '__main__':
    unittest.main()

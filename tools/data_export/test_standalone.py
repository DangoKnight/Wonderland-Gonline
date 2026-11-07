"""Standalone extraction must dispatch native tools without invoking Go."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from export import tool_command
from regenerate import regenerate


class StandaloneTests(unittest.TestCase):
    def test_linked_tools_preserve_argument_boundaries(self):
        with patch.dict(os.environ, {'WLO_ASSET_TOOL': 'C:/My Game/client.exe'}):
            self.assertEqual(tool_command('sprite-export', '-input', 'D:/Legacy Client/jma'),
                             ['C:/My Game/client.exe', '-asset-tool', 'sprite-export',
                              '-input', 'D:/Legacy Client/jma'])

    def test_checkout_tools_remain_available(self):
        with patch.dict(os.environ, clear=True):
            self.assertEqual(tool_command('item-export', '-input', 'Item.dat'),
                             ['go', 'run', './cmd/item-export', '-input', 'Item.dat'])

    def test_complete_regeneration_uses_linked_exporters(self):
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {'WLO_ASSET_TOOL': 'client.exe'}):
            root = Path(temp)
            source, output = root / 'Legacy Client', root / 'extracted'
            for directory in ('data', 'jma', 'pic'):
                (source / directory).mkdir(parents=True)
            (source / 'aLogin.exe').write_bytes(b'fixture')
            with patch('regenerate.export_all') as tables, \
                    patch('regenerate.annotate_languages') as languages, \
                    patch('regenerate.preserve_server_exports'), \
                    patch('regenerate.subprocess.run') as run, \
                    patch('regenerate.export_media') as media, \
                    patch('regenerate.validate_output_formats', return_value={}):
                regenerate(output, source)
            self.assertEqual([call.args[0][0:3] for call in run.call_args_list],
                             [['client.exe', '-asset-tool', 'sprite-export'],
                              ['client.exe', '-asset-tool', 'pic-export']])
            tables.assert_called_once()
            languages.assert_called_once_with(output.resolve(), 'en', None)
            media.assert_called_once()
            self.assertTrue((output / 'external_asset_import.json').is_file())


if __name__ == '__main__':
    unittest.main()

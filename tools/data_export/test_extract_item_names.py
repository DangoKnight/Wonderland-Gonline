import json
from pathlib import Path
import tempfile
import unittest

import extract_item_names as names


class ItemNamesTest(unittest.TestCase):
    def test_saved_table_and_duplicate_detection(self):
        with tempfile.TemporaryDirectory() as directory:
            state = json.dumps({'data': {'search': '', 'paginators': [{'page': 1}]}}).replace('"', '&quot;')
            page = f'<div wire:snapshot="{state}"><tr wire:key="item-42"><td><img src="01_files/123_hash.png"></td><td>Full &amp; Name (Quest)</td><td>12</td><td>—</td><td>Armor <span class="subtype-text">(Head)</span></td><td>Gold</td><td>Def +4</td><td>Some description.</td></tr></div>'
            Path(directory, '01.htm').write_text(page, encoding='utf-8')
            rows, sources = names.extract(Path(directory))
            self.assertEqual((rows[0]['name'], rows[0]['icon'], rows[0]['rank'], rows[0]['subtype']), ('Full & Name', 123, None, 'Head'))
            self.assertEqual(sources[0]['records'], 1)
            Path(directory, '02.htm').write_text(page, encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'duplicate item'):
                names.extract(Path(directory))

    def test_icon_rank_and_description_matching(self):
        native = dict(definition=dict(id=1, name='Pure Iron Knif'), icon=42, decoded_hex=(bytes(45)+bytes([5])+bytes(405)).hex(), description='A knife')
        item = dict(source_id=99, name='Pure Iron Knife', icon=42, rank=5, description='A knife')
        patches, review = names.match([item], [native])
        self.assertEqual(patches[0]['name'], 'Pure Iron Knife')
        patches, review = names.match([dict(item, icon=43)], [native])
        self.assertFalse(patches)
        self.assertEqual(review[0]['reason'], 'icon/rank mismatch')
        patches, review = names.match([item, dict(item, source_id=100, name='Pure Iron Knife+1', description='Another knife')], [native])
        self.assertEqual(patches[0]['name'], 'Pure Iron Knife')

    def test_complete_names_and_enhancement_prefixes(self):
        native = dict(definition=dict(id=1, name='Golden Rings+1'), icon=42, decoded_hex=bytes(451).hex(), description='')
        item = dict(source_id=99, name='Golden Rings+10', icon=42, rank=0, description='')
        patches, review = names.match([item], [native])
        self.assertFalse(patches)
        self.assertEqual(len(review), 1)
        native['definition']['name'] = 'Polished Wand'
        item['name'] = 'Polished Wand'
        patches, review = names.match([item, dict(item, source_id=100, name='Polished Wand+1')], [native])
        self.assertFalse(patches)
        self.assertFalse(review)


if __name__ == '__main__':
    unittest.main()

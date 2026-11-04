import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

import extract_monster_names as names


class MonsterNamesTest(unittest.TestCase):
    def test_owner_review_can_shorten_names_and_preserve_confirmed_names(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, 'npcs.json')
            def native(id, name):
                return dict(fields=dict(id=id,level=1,element=1,hp=1,sp=1,strength=1,constitution=1,intelligence=1,wisdom=1,agility=1),name=dict(text=name))
            path.write_text(json.dumps(dict(records=[native(1,'Old Dragon'),native(2,'Bear')])),encoding='utf-8')
            manual={1:dict(original_name='Old Dragon',name='Old Man'),2:dict(original_name='Bear',name='Bear')}
            patches, review=names.match([],npc_export=path,manual=manual)
            self.assertEqual(patches,[dict(id=1,original_name='Old Dragon',name='Old Man',source_ids='owner-review',previous_name='')])
            self.assertFalse(review)
            with self.assertRaisesRegex(ValueError,'native name changed'):
                names.match([],npc_export=path,manual={1:dict(original_name='Changed',name='Old Man')})

    def test_reviewed_nonprefix_names_guard_native_and_source_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, 'npcs.json')
            fields = dict(id=17466, level=5, element=1, hp=62, sp=100, strength=4, constitution=4, intelligence=1, wisdom=1, agility=5)
            path.write_text(json.dumps(dict(records=[dict(fields=fields, name=dict(text='Snake'))])), encoding='utf-8')
            monster = dict(source_id=1080, name='Poisonous Snake', lvl=5, element=1, hp=62, sp=100, str=4, con=4, int=1, wis=1, agi=5)
            decision = dict(original_name='Snake', name='Poisonous Snake', source_ids='1080', method='exact-species', notes='Snake model and exact attributes.', native_level='5', native_element='1', native_stats='4;4;1;1;5')
            patches, review = names.match([monster], npc_export=path, reviewed={17466: decision})
            self.assertEqual([(r['id'], r['name']) for r in patches], [(17466, 'Poisonous Snake')])
            self.assertFalse(review)
            for change, error in [(dict(original_name='Bear'), 'native name changed'), (dict(native_level='6'), 'native identity changed'), (dict(source_ids='999'), 'invalid source identity'), (dict(method='exact-prefix'), 'prefix changed')]:
                with self.assertRaisesRegex(ValueError, error):
                    names.match([monster], npc_export=path, reviewed={17466: dict(decision, **change)})
            with self.assertRaisesRegex(ValueError, 'exact metadata changed'):
                names.match([dict(monster, con=5)], npc_export=path, reviewed={17466: decision})
            self.assertTrue(names.match([monster], npc_export=path, reviewed={17466: dict(decision, method='exact-vitals')})[0])
            with self.assertRaisesRegex(ValueError, 'exact vitals changed'):
                names.match([dict(monster, hp=63)], npc_export=path, reviewed={17466: dict(decision, method='exact-vitals')})

    def test_html_entities_and_stats(self):
        with tempfile.TemporaryDirectory() as directory:
            state = json.dumps({'data': {'paginators': [{'page': 1}]}}).replace('"', '&quot;')
            stats = ''.join(f'<span class="stat-title">{k}</span> <span class="stat-val">{1 if k != "Catch" else "Yes"}</span>' for k in names.STAT_NAMES)
            page = f'<div wire:snapshot="{state}"><div class="monster-card" wire:key="monster-42"><h3 class="monster-name">Wolf &amp;  Cub</h3><img class="element-icon" title="Wind">{stats}</div></div>'
            Path(directory, 'page.htm').write_text(page, encoding='utf-8')
            rows, sources = names.extract(Path(directory))
            self.assertEqual((rows[0]['name'], rows[0]['element'], rows[0]['lvl']), ('Wolf & Cub', 4, 1))
            self.assertEqual(sources[0]['records'], 1)
            Path(directory, 'duplicate.htm').write_text(page, encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'duplicate monster'):
                names.extract(Path(directory))

    def test_matching_does_not_guess_ambiguous_or_mismatched_rows(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, 'assets.db')
            with sqlite3.connect(path) as db:
                db.execute('CREATE TABLE catalog_n_p_cs(value_id INTEGER,value_name TEXT,value_level INTEGER,value_element INTEGER,value_stats0 INTEGER,value_stats1 INTEGER,value_stats2 INTEGER,value_stats3 INTEGER,value_stats4 INTEGER)')
                db.executemany('INSERT INTO catalog_n_p_cs VALUES(?,?,?,?,?,?,?,?,?)', [(1, 'Blue Beetl', 1, 4, 1, 2, 3, 4, 5), (2, 'Blue Beetl', 2, 4, 1, 2, 3, 4, 5), (3, 'Custom', 1, 4, 1, 2, 3, 4, 5)])
                db.execute('INSERT INTO catalog_n_p_cs VALUES(4,?,?,?,?,?,?,?,?)', ('Pineapple', 1, 4, 1, 2, 3, 4, 5))
            # sqlite3's context manager commits but does not close on Windows.
            db.close()
            monster = dict(source_id=99, name='Blue Beetle', lvl=1, element=4, str=1, con=2, int=3, wis=4, agi=5)
            patches, review = names.match([monster], path)
            self.assertEqual([(p['id'], p['name']) for p in patches], [(1, 'Blue Beetle')])
            self.assertEqual([r['id'] for r in review], [2])
            patches, review = names.match([dict(monster, name='Blue Beetle (Quest)')], path, accept_single=True, strip_quest=True)
            self.assertEqual([p['id'] for p in patches], [1, 2])
            self.assertTrue(all(p['name'] == 'Blue Beetle' and p['previous_name'] == 'Blue Beetle (Quest)' for p in patches))
            self.assertFalse(review)
            patches, review = names.match([monster, dict(monster, source_id=100, name='Blue Beetle (Quest)')], path, accept_single=True, strip_quest=True)
            self.assertEqual(len(patches), 2)
            self.assertFalse(review)
            patches, review = names.match([monster, dict(monster, source_id=100, name='Blue Beetle King')], path)
            self.assertFalse(patches)
            self.assertEqual(review[0]['reason'], 'ambiguous')
            patches, review = names.match([monster, dict(monster, source_id=100, name='Blue Beetle King')], path, choices={1: 'Blue Beetle King', 2: 'Blue Beetle'})
            self.assertEqual([p['name'] for p in patches], ['Blue Beetle King', 'Blue Beetle'])
            self.assertFalse(review)
            with self.assertRaisesRegex(ValueError, 'not a reference candidate'):
                names.match([monster], path, choices={1: 'Wrong monster'})
            pineapple = dict(monster, name='Pineapple Monster')
            self.assertFalse(names.match([pineapple], path)[0])
            patches, review = names.match([pineapple], path, choices={4: 'Pineapple Monster'})
            self.assertEqual([(p['id'], p['name']) for p in patches], [(4, 'Pineapple Monster')])


if __name__ == '__main__':
    unittest.main()

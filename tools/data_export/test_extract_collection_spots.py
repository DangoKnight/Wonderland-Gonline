import json
from pathlib import Path
import tempfile
import unittest

import extract_collection_spots as collections


class CollectionSpotsTest(unittest.TestCase):
    def test_page_parse_and_vacuum_classification(self):
        with tempfile.TemporaryDirectory() as directory:
            state=json.dumps({'data':{'paginators':[{'page':1}]}}).replace('"','&quot;')
            rewards=''.join(f'<a href="https://wlohub.com/items/{i}"><span class="ing-name">Item {i}</span></a>' for i in (1,2))
            page=f'<div wire:snapshot="{state}"><tr wire:key="col-1"><span class="loc-name">North Island</span><span class="coordinate-tag">X: 5300</span><span class="coordinate-tag">Y: 1900</span>{rewards}</tr></div>'
            Path(directory,'01.htm').write_text(page,encoding='utf-8')
            spots,sources=collections.extract(Path(directory))
            self.assertEqual((spots[1]['kind'],spots[1]['x'],spots[1]['y']),('vacuum',5300,1900))
            self.assertEqual(sources[0]['records'],1)
            Path(directory,'02.htm').write_text(page,encoding='utf-8')
            with self.assertRaisesRegex(ValueError,'duplicate collection'):
                collections.extract(Path(directory))

    def test_bounds_do_not_force_ambiguous_maps_or_discard_good_rows(self):
        scenes=[dict(fields=dict(id=i),name=dict(text='Village')) for i in (1,2)]
        spots={1:dict(location='Village',region='',position_known=True,x=50,y=60),2:dict(location='Village',region='',position_known=True,x=500,y=600)}
        collections.resolve_maps(spots,scenes,{1:(100,100),2:(200,200)},{})
        self.assertEqual(spots[1]['map_id'],0)
        self.assertEqual(spots[1]['map_candidates'],[1,2])
        self.assertFalse(spots[2]['map_candidates'])

    def test_reward_mapping(self):
        spots={1:dict(rewards=[dict(source_item_id=99)])}
        ref=[dict(source_id='99',name='White Clay',icon='42',rank='1')]
        row=dict(definition=dict(id=44004,name='White Clay'),icon=42,decoded_hex=(bytes(45)+bytes([1])+bytes(405)).hex())
        mapping,_=collections.resolve_items(spots,ref,[row],{})
        self.assertEqual(mapping,{99:44004})
        mapping,_=collections.resolve_items(spots,ref,[row,dict(row,definition=dict(id=44005,name='White Clay'))],{})
        self.assertEqual(mapping,{99:0})


if __name__=='__main__':
    unittest.main()

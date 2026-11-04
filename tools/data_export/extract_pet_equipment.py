#!/usr/bin/env python3
"""Extract pet special equipment; seed normal acquisition equipment only."""
import argparse
import hashlib
import html
import json
from pathlib import Path
import re

from extract_monster_names import text, write_csv
from extract_collection_spots import csv_rows

REFERENCE=Path(__file__).parent/'reference'/'wlohub-pets'
NATIVE_RANK_OFFSET=45
MAX_EQUIPMENT_SLOT=6


def extract(pages):
    pets={};sources=[]
    for page in sorted(pages.glob('*.htm*')):
        raw=page.read_bytes();source=raw.decode('utf-8-sig')
        for sid,row in re.findall(r'<tr[^>]*wire:key="pet-(\d+)"[^>]*>(.*?)</tr>',source,re.S):
            cells=re.findall(r'<td\b[^>]*>(.*?)</td>',row,re.S)
            name=re.search(r'class="pet-name-link">(.*?)</a>',row,re.S)
            if name is None or len(cells)<2:raise ValueError(f'invalid pet {sid}')
            sections=re.split(r'>\s*(Normal|Reborn):\s*</div>',cells[1])
            equipment={}
            for i in range(1,len(sections),2):
                ids=re.findall(r'<a href="https://wlohub.com/items/(\d+)"',sections[i+1])
                if len(ids)>1:raise ValueError(f'pet {sid}: multiple {sections[i]} items')
                equipment[sections[i].lower()]=int(ids[0]) if ids else 0
            if set(equipment)!={'normal','reborn'}:raise ValueError(f'pet {sid}: missing stages')
            if int(sid) in pets:raise ValueError(f'duplicate pet {sid}')
            pets[int(sid)]=dict(name=text(name[1]),**equipment)
        sources.append(dict(file=page.name,sha256=hashlib.sha256(raw).hexdigest()))
    if not pets:raise ValueError('no pets found')
    return pets,sources


def resolve(ref,native,choice=0):
    candidates=[r for r in native if (r['definition']['name'].casefold()==ref['name'].casefold() or len(r['definition']['name'])>=13 and ref['name'].startswith(r['definition']['name'])) and str(r['icon'])==ref['icon'] and str(bytes.fromhex(r['decoded_hex'])[NATIVE_RANK_OFFSET])==ref['rank']]
    if choice:
        candidates=[r for r in native if r['definition']['id']==choice and str(r['icon'])==ref['icon'] and str(bytes.fromhex(r['decoded_hex'])[NATIVE_RANK_OFFSET])==ref['rank']]
        if len(candidates)!=1:raise ValueError('invalid authored item choice')
    return candidates[0] if len(candidates)==1 else None


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ('pages','item-export','output-dir'):p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--item-reference',type=Path,default=Path(__file__).parent/'reference'/'wlohub-items'/'items.csv')
    p.add_argument('--templates',type=Path,default=REFERENCE/'templates.csv')
    p.add_argument('--item-choices',type=Path,default=REFERENCE/'item_choices.csv')
    a=p.parse_args();pets,sources=extract(a.pages)
    refs={int(r['source_id']):r for r in csv_rows(a.item_reference)}
    choices={int(r['source_item_id']):int(r['item_id']) for r in csv_rows(a.item_choices)}
    templates={int(r['source_pet_id']):r for r in csv_rows(a.templates)}
    native=json.loads(a.item_export.read_text(encoding='utf-8'))['items']
    defaults=[];report=[]
    for sid,pet in sorted(pets.items()):
        mapping=templates[sid]
        if pet['name']!=mapping['name']:raise ValueError(f'pet identity mismatch {sid}')
        for stage in ('normal','reborn'):
            wid=pet[stage];item=resolve(refs[wid],native,choices.get(wid,0)) if wid else None
            valid=item is not None and 1<=item['definition']['equip_slot']<=MAX_EQUIPMENT_SLOT
            enabled=stage=='normal' and valid
            report.append(dict(source_pet_id=sid,template_id=mapping['template_id'],pet=pet['name'],stage=stage,source_item_id=wid or '',name=refs[wid]['name'] if wid else '',item_id=item['definition']['id'] if item else '',equipment_slot=item['definition']['equip_slot'] if item else '',grant_on_acquisition=enabled,status='no item listed' if not wid else 'not equipment' if item is not None and not valid else 'unresolved' if item is None else 'resolved'))
            if enabled:
                defaults.append(dict(template=int(mapping['template_id']),equipment=[dict(slot=item['definition']['equip_slot']-1,item_id=item['definition']['id'])],allow_missing=True))
    write_csv(a.output_dir/'equipment.csv',report,['source_pet_id','template_id','pet','stage','source_item_id','name','item_id','equipment_slot','grant_on_acquisition','status'])
    (a.output_dir/'pet_special_equipment_defaults.json').write_bytes((json.dumps(defaults,indent=2)+'\n').encode())
    manifest=dict(source='https://wlohub.com/pets',pages=sources,pets=len(pets),starter_definitions=len(defaults),normal_only=True)
    for key in ('item_export','item_reference','templates','item_choices'):manifest[key+'_sha256']=hashlib.sha256(getattr(a,key).read_bytes()).hexdigest()
    (a.output_dir/'sources.json').write_bytes((json.dumps(manifest,indent=2)+'\n').encode());print(json.dumps({k:v for k,v in manifest.items() if k not in ('pages',) and not k.endswith('_sha256')}))


if __name__=='__main__':main()

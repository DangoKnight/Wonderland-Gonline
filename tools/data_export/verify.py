#!/usr/bin/env python3
"""Reconstruct original bytes from exports and verify the complete source inventory."""
import argparse
import hashlib
import json
from pathlib import Path
import struct

from export import (FORMATS, REPO, inventory, require, sha256, transform,
                    COPY_BUFFER_BYTES, EVE_HEADER_BYTES)

# Independent item wire map, retained as data for inverse verification of the
# Go exporter. The native item golden tests separately check decoded meaning.
ITEM_INVERSE = dict(prefix=0, bias=9, masks={1:0x9a,2:0xefc3,4:0x0b80f4b4},
    reversals=[(1,15),(147,401)], offsets={
        1:[15,34,35,44,45,46,47,112,113,122,129,401,402,403,406,407,408,415,416,417,418,419,420],
        2:[16,18,20,22,24,26,28,30,32,123,134,136,138,140,142,144,404,409,411,413,421,423,425,427,429],
        4:[36,40,48,52,56,60,64,68,72,76,80,84,88,92,96,100,104,108,114,118,125,130,431,435,439,443,447]})


def reconstruct(document, asset):
    if asset=='item.dat':
        return bytes.fromhex(document['header_hex']) + b''.join(
            transform(bytes.fromhex(row['decoded_hex']),ITEM_INVERSE,encrypt=True)
            for row in document['items'])
    kind=document['format']
    if kind=='native-records':
        return bytes.fromhex(document['header_hex']) + b''.join(
            transform(bytes.fromhex(row['decoded_hex']),FORMATS[asset],encrypt=True)
            for row in document['records'])
    if kind in ('plaintext-named-archive','plaintext-animation-archive'):
        rows=document['entries'] if 'entries' in document else document['records']
        return (b''.join(bytes.fromhex(row['decoded_hex']) for row in rows)
                +b''.join(bytes.fromhex(row['index_hex']) for row in rows)
                +bytes.fromhex(document['footer_count_hex']))
    if kind=='plaintext-eve':
        result=bytearray(document['source_bytes'])
        result[:EVE_HEADER_BYTES]=bytes.fromhex(document['header_hex'])
        position=EVE_HEADER_BYTES
        for row in document['maps']:
            native_index=bytes.fromhex(row['index_hex'])
            result[position:position+len(native_index)]=native_index
            position+=len(native_index)
            block=bytes.fromhex(row['decoded_hex'])
            offset=row['file_offset'];result[offset:offset+len(block)]=block
        for gap in document['gaps']:
            block=bytes.fromhex(gap['decoded_hex'])
            offset=gap['file_offset'];result[offset:offset+len(block)]=block
        return bytes(result)
    if kind=='plaintext-converted-items':
        return b''.join(bytes.fromhex(row['decoded_hex']) for row in document['records'])
    if 'decoded_hex' in document:
        return bytes.fromhex(document['decoded_hex'])
    return bytes.fromhex(document['source_hex'])


def check_readable(document):
    if document.get('format')=='native-records':
        for row in document['records']:
            record=bytes.fromhex(row['decoded_hex'])
            for field in document['field_layout']:
                offset=field['offset'];width=field['bytes']
                require(row['fields'][field['name']]==int.from_bytes(record[offset:offset+width],'little'), 'readable field differs from decoded bytes')
    if document.get('format')=='plaintext-formula':
        encoded=(bytes([document['version']])
                 +struct.pack('<'+'d'*len(document['coefficients']),*document['coefficients'])
                 +struct.pack('<I',document['level_scaler'])
                 +struct.pack('<'+'H'*len(document['level_gates']),*document['level_gates']))
        require(encoded.hex()==document['decoded_hex'],'readable formula differs from original bytes')


def verify(destination):
    manifest=json.loads((destination/'asset_manifest.json').read_text())
    selected,shadowed=inventory(Path(manifest['client_source']),Path(manifest['server_source']))
    entries={entry['asset']:entry for entry in manifest['assets']}
    require(len(entries)==len(manifest['assets']) and entries.keys()==selected.keys() | (entries.keys() & {'skill_effects.json', 'lucky_draw.json'}),'manifest inventory is incomplete or duplicated')
    require(manifest['priority']=='client-over-server','incorrect source priority')
    if 'lucky_draw.json' in entries:
        from lucky_draw import RULES
        row = entries['lucky_draw.json']
        path = destination / row['output']
        lucky = json.loads(path.read_text())
        require(sha256(path) == row['output_sha256'] and path.stat().st_size == row['output_bytes'], 'Lucky Draw export checksum mismatch')
        require(lucky['source_sha256'] == entries['item.dat']['source_sha256'] == row['source_sha256'], 'Lucky Draw source mismatch')
        require(lucky['policy_sha256'] == sha256(RULES), 'Lucky Draw policy changed; regenerate')
        policy = json.loads(RULES.read_text())
        items = {r['definition']['id']: r['definition'] for r in json.loads((destination / entries['item.dat']['output']).read_text())['items']}
        expected = [{**reward, 'name': items[reward['item_id']]['name']} for reward in policy['rewards']]
        require(lucky['value'] == expected and lucky['format'] == 'lucky-draw-rewards', 'Lucky Draw reward projection differs from policy/source')
    if 'skill_effects.json' in entries:
        effect_entry = entries['skill_effects.json']
        effect_path = destination / effect_entry['output']
        require(sha256(effect_path) == effect_entry['output_sha256'] and effect_path.stat().st_size == effect_entry['output_bytes'], 'effect export checksum mismatch')
        effects = json.loads(effect_path.read_text())
        require(effects['source_sha256'] == entries['skill.dat']['source_sha256'] == effect_entry['source_sha256'], 'effect source provenance mismatch')
        require(effects['schema_version'] == 1 and effects['format'] == 'skill-effect-definitions', 'unsupported effect definitions')
        ids = [r['id'] for r in effects['records']]
        require(len(ids) == len(set(ids)), 'duplicate effect identifiers')
        skills = json.loads((destination / entries['skill.dat']['output']).read_text())
        for row in skills['records']:
            require(all(ref in ids for ref in row['fields']['effect_refs']), 'dangling skill effect reference')

    for asset,(origin,source) in sorted(selected.items()):
        entry=entries[asset]
        require(entry['origin']==origin and Path(entry['source'])==source,'wrong source selected: '+asset)
        require(sha256(source)==entry['source_sha256'],'source checksum mismatch: '+asset)
        output=destination/entry['output']
        require(sha256(output)==entry['output_sha256'] and output.stat().st_size==entry['output_bytes'],'export checksum mismatch: '+asset)
        document=json.loads(output.read_text())
        require(document['source_sha256']==entry['source_sha256'],'embedded provenance mismatch')
        if asset in shadowed:
            require(sha256(shadowed[asset])==entry['overridden_server_source']['sha256'],'overridden source checksum mismatch')
        check_readable(document)
        if 'payload' in entry or 'audio_tracks' in entry:
            descriptor=entry.get('audio_tracks',entry.get('payload'))
            payload=destination/entry['payload']['output'] if 'payload' in entry else None
            digest=hashlib.sha256()
            payload_bytes=0
            if payload is not None and payload.exists():
                with payload.open('rb') as stream:
                    for chunk in iter(lambda:stream.read(COPY_BUFFER_BYTES),b''):
                        digest.update(chunk)
                        payload_bytes+=len(chunk)
            else:
                # Individual OGG tracks replace the combined payload locally.
                directory=destination/descriptor['directory'] if 'audio_tracks' in entry else payload.parent/payload.stem
                for row in document['entries']:
                    name=row['name']
                    require(Path(name).name==name and '/' not in name and '\\' not in name,'unsafe audio name')
                    require(row['file_offset']==payload_bytes,'noncontiguous audio index')
                    track=directory/name
                    require(track.stat().st_size==row['bytes'],'audio track size mismatch')
                    with track.open('rb') as stream:
                        for chunk in iter(lambda:stream.read(COPY_BUFFER_BYTES),b''):
                            digest.update(chunk)
                            payload_bytes+=len(chunk)
            require(digest.hexdigest()==descriptor['sha256']==document['payload_sha256'],'audio payload checksum mismatch')
            require(payload_bytes==descriptor['bytes']==document['payload_bytes'],'audio size mismatch')
            digest.update(bytes.fromhex(document['footer_hex']))
            require(digest.hexdigest()==entry['source_sha256'],'audio reconstruction mismatch')
        else:
            reconstructed=reconstruct(document,asset)
            require(len(reconstructed)==entry['source_bytes'] and hashlib.sha256(reconstructed).hexdigest()==entry['source_sha256'],'source reconstruction mismatch: '+asset)
        print(asset+': source selection, export checksum and complete reconstruction verified',flush=True)
    print(f'All {len(selected)} source assets reconstruct byte-for-byte; derived compatibility definitions verified separately.',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data',type=Path,default=REPO/'data')
    verify(parser.parse_args().data.resolve())

#!/usr/bin/env python3
"""Lossless offline export of Wonderland resources; no runtime dependency on Python."""
import argparse
import csv
import hashlib
import io
import json
import mmap
import os
from pathlib import Path
import struct
import subprocess
import tempfile
import xml.etree.ElementTree as ET

REPO = Path(__file__).resolve().parents[2]
SCHEMA_VERSION = 1
NAMED_INDEX_BYTES = 29
NAMED_NAME_BYTES = 21
INDEX_COUNT_BYTES = 2
ANIMATION_INDEX_BYTES = 10
ANIMATION_MIN_RECORD_BYTES = 27
EVE_HEADER_BYTES = 12
EVE_INDEX_BYTES = 10
EVE_SECTION_COUNT = 11
EVE_SECTION_NAMES = ('npcs', 'ground_items', 'warps', 'events', 'pre_events',
                     'entries', 'mining', 'groups', 'interactive', 'battles', 'extended_groups')
FORMULA_VERSION = 2
FORMULA_COEFFICIENT_COUNT = 45
FORMULA_GATE_COUNT = 21
CONVERTED_ITEM_BYTES = 47
OGG_HEADER_BYTES = 27
OGG_BEGIN_STREAM = 2
OGG_END_STREAM = 4
COPY_BUFFER_BYTES = 1 << 20
GROUND_GRID_LIMIT = 900


def u16(data, offset=0):
    return struct.unpack_from('<H', data, offset)[0]


def u32(data, offset=0):
    return struct.unpack_from('<I', data, offset)[0]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(COPY_BUFFER_BYTES), b''):
            digest.update(chunk)
    return digest.hexdigest()


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix='.export-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'w', encoding='utf-8') as stream:
            json.dump(value, stream, ensure_ascii=False, indent=2, allow_nan=False)
            stream.write('\n')
        os.chmod(temporary, 0o644)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def copy_payload(source, output, size):
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix='.export-', dir=output.parent)
    try:
        with source.open('rb') as stream, os.fdopen(descriptor, 'wb') as target:
            remaining = size
            while remaining:
                chunk = stream.read(min(remaining, COPY_BUFFER_BYTES))
                require(chunk, 'truncated source during copy')
                target.write(chunk)
                remaining -= len(chunk)
        os.chmod(temporary, 0o644)
        os.replace(temporary, output)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def readable_text(raw):
    raw = raw.rstrip(b'\x00 ')
    try:
        return raw.decode('utf-8'), 'utf-8'
    except UnicodeDecodeError:
        try:
            return raw.decode('big5'), 'big5'
        except UnicodeDecodeError:
            # Original bytes remain in decoded_hex/source_hex; never lose data.
            return raw.decode('big5', errors='replace'), 'big5-with-replacement'


def short_text(record, length_offset, start, capacity):
    length = record[length_offset]
    text, encoding = readable_text(record[start:start + min(length, capacity)])
    return {'text': text, 'encoding': encoding, 'length': length,
            'length_exceeds_capacity': length > capacity}


# Native object offsets below include the loader's prefix. Disk offsets are
# derived explicitly, including SceneData's six-byte prefix. Unresolved fields
# retain stable offset names. These are format tables, not gameplay constants.
FORMATS = {
    'npc.dat': dict(size=138, prefix=4, version=(38, 4, 12), bias=1,
        masks={1: 0xc8, 2: 0x5209, 4: 0x0baeb716},
        offsets={1: [0x0f,0x26,0x27,0x28,0x29,0x3c,0x3d,0x4e,0x57,0x5a,0x5b,0x66,0x67,0x68,0x6b,0x6c,0x6d,0x6e,0x6f],
                 2: [0x10,0x12,0x14,0x32,0x34,0x36,0x38,0x3a,0x3e,0x40,0x42,0x44,0x46,0x48,0x4a,0x4c,0x4f,0x51,0x53,0x55,0x58,0x5c,0x5e,0x60,0x62,0x64,0x69,0x70,0x72,0x74,0x76,0x78],
                 4: [0x16,0x1a,0x1e,0x22,0x2a,0x2e,0x7a,0x7e,0x82,0x86,0x8a]},
        reversals=[(1, 11)], texts={'name': (0, 1, 10)},
        names={11:'type',12:'id',37:'level',38:'hp',42:'sp',57:'element',46:'strength',48:'constitution',50:'intelligence',52:'wisdom',54:'agility',58:'skill_1',60:'skill_2',62:'skill_3',64:'drop_1',66:'drop_2',68:'drop_3',70:'drop_4',72:'drop_5',90:'book_index'},
        output='npc_data.json', reference='aLogin FUN_00481cac / FUN_00481814'),
    'skill.dat': dict(size=148, prefix=4, version=(21, 1, 11), bias=4,
        masks={1: 0xfd, 2: 0x6ea0, 4: 0x0bdedebf},
        offsets={1: [0x19,0x1e,0x21,0x22,0x23,0x34,0x37,0x38,0x67,0x6c,0x6d,0x6e,0x71,0x72,0x73,0x74,0x75,0x76,0x77,0x78,0x79],
                 2: [0x1a,0x1c,0x1f,0x35,0x3a,0x3c,0x3e,0x40,0x42,0x44,0x65,0x68,0x6a,0x6f,0x7a,0x7c,0x7e,0x80,0x82],
                 4: [0x84,0x88,0x8c,0x90,0x94]},
        reversals=[(1,21),(67,97)], texts={'name': (0,1,20), 'description': (66,67,30)},
        names={21:'type',22:'id',24:'sp',26:'element',27:'attack_category',29:'effect_layer',49:'additional_damage',97:'table_order'},
        output='skill_data.json', reference='aLogin FUN_00480f8c / FUN_00480b80'),
    'mark.dat': dict(size=553, prefix=4, version=(258, 2, 4), bias=7,
        masks={1: 0x90, 2: 0x2774, 4: 0x082cd1b9},
        offsets={1:[0x103,0x108,0x208,0x209,0x20a,0x20b,0x20c,0x20d,0x20e],
                 2:[0x104,0x106,0x20f,0x211,0x213,0x215,0x217],
                 4:[0x219,0x21d,0x221,0x225,0x229]},
        reversals=[(1,255),(262,516)], texts={'name':(0,1,254),'description':(261,262,254)},
        names={256:'id',258:'completion_flag'}, output='mark_data.json', reference='aLogin FUN_00406300 / FUN_00405fc0'),
    'talk.dat': dict(size=292, prefix=4, version=(0, 2, 3), bias=5,
        masks={1:0x63,2:0xecea,4:0x07e00b30},
        offsets={1:[0x105,0x106,0x107,0x108,0x109],
                 2:[4,0x10a,0x10c,0x10e,0x110,0x112],
                 4:[0x114,0x118,0x11c,0x120,0x124]},
        reversals=[(3,257)], texts={'text':(2,3,254)}, names={0:'id'},
        output='talk_data.json', reference='aLogin FUN_00333360 (before content substitutions)'),
    'scenedata.dat': dict(size=131, prefix=6, version=(0, 2, 4), bias=9,
        masks={1:0x2c,2:0xea6c,4:0x062b7ba7},
        offsets={1:[0x1d,0x1e,0x27,0x28,0x29,0x66,0x67,0x68,0x69,0x6a],
                 2:[6,*range(0x2c,0x40,2),*range(0x42,0x56,2),0x58,0x5a,0x5c,0x5e,0x60,0x62,0x64,0x6b,0x6d,0x6f,0x71,0x73],
                 4:[0x75,0x79,0x7d,0x81,0x85]},
        reversals=[(3,23),(26,33)], texts={'name':(2,3,20),'secondary_name':(25,26,7)}, names={0:'id'},
        output='scene_data.json', reference='aLogin FUN_0047be24 / FUN_0047c808 (before content patches)'),
    'compound.dat': dict(size=65, prefix=0, version=None, bias=3,
        masks={1:0xd3,2:0xfbbc,4:0x0a06f965},
        offsets={1:[4,7,13,16,19,22,25,26,*range(29,35)],
                 2:[0,2,5,11,14,17,20,23,27,35,37,39,41,43],4:[45,49,53,57,61]},
        reversals=[], texts={}, names={0:'result_id',2:'plan_id',5:'tool_id',7:'result_count',11:'material_1_id',13:'material_1_count',14:'material_2_id',16:'material_2_count',17:'material_3_id',19:'material_3_count',20:'material_4_id',22:'material_4_count',23:'material_5_id',25:'material_5_count',27:'build_time'},
        output='compound_data.json', reference='Wonderland-Private-Server cBuildElement.Load'),
}
FORMATS['compound2.dat'] = dict(FORMATS['compound.dat'], output='compound2_data.json')


def transform(record, spec, encrypt=False):
    result = bytearray(record)
    if not encrypt:
        for start, end in spec['reversals']:
            result[start:end] = result[start:end][::-1]
    for width, addresses in spec['offsets'].items():
        mask = spec['masks'][width]
        limit = (1 << (width * 8)) - 1
        for address in addresses:
            offset = address - spec['prefix']
            value = int.from_bytes(result[offset:offset+width], 'little')
            value = ((value + spec['bias']) & limit) ^ mask if encrypt else ((value ^ mask) - spec['bias']) & limit
            result[offset:offset+width] = value.to_bytes(width, 'little')
    if encrypt:
        for start, end in spec['reversals']:
            result[start:end] = result[start:end][::-1]
    return bytes(result)


def export_records(data, spec):
    require(data and len(data) % spec['size'] == 0, 'invalid native record length')
    occupied = set()
    layout = []
    for width, addresses in spec['offsets'].items():
        for address in addresses:
            offset = address - spec['prefix']
            require(0 <= offset <= spec['size']-width, 'cipher field outside record')
            span = set(range(offset, offset+width))
            require(not occupied & span, 'overlapping cipher fields')
            occupied |= span
            layout.append({'name': spec['names'].get(offset, f'unknown_u{width*8}_offset_{offset}'),
                           'offset': offset, 'bytes': width})
    layout.sort(key=lambda field: field['offset'])
    header_bytes = spec['size'] if spec['version'] else 0
    if spec['version']:
        offset, width, version = spec['version']
        require(int.from_bytes(data[offset:offset+width], 'little') == version, 'unsupported native table version')
    records = []
    for offset in range(header_bytes, len(data), spec['size']):
        source = data[offset:offset+spec['size']]
        decoded = transform(source, spec)
        require(transform(decoded, spec, encrypt=True) == source, 'native record round trip failed')
        fields = {field['name']: int.from_bytes(decoded[field['offset']:field['offset']+field['bytes']], 'little') for field in layout}
        texts = {name: short_text(decoded, *positions) for name, positions in spec['texts'].items()}
        if spec['output'] == 'skill_data.json':
            fields['power_per_level'], fields['stat_multiplier'] = struct.unpack_from('<dd', decoded, 32)
        records.append({'record_index': offset // spec['size'], 'file_offset': offset,
                        **texts, 'fields': fields, 'decoded_hex': decoded.hex()})
    return {'format': 'native-records', 'record_bytes': spec['size'], 'native_version': spec['version'][2] if spec['version'] else None,
            'decoder_reference': spec['reference'], 'header_hex': data[:header_bytes].hex(),
            'field_layout': layout, 'records': records}


def named_index(data):
    require(len(data) >= INDEX_COUNT_BYTES, 'truncated archive')
    count = u16(data, len(data)-INDEX_COUNT_BYTES)
    start = len(data) - INDEX_COUNT_BYTES - count * NAMED_INDEX_BYTES
    require(count and start >= 0, 'invalid named archive count')
    entries = []
    previous = 0
    for index in range(count):
        position = start + index * NAMED_INDEX_BYTES
        row = data[position:position+NAMED_INDEX_BYTES]
        require(row[0] < NAMED_NAME_BYTES, 'archive filename too long')
        name, encoding = readable_text(row[1:1+row[0]])
        offset, size = struct.unpack_from('<II', row, NAMED_NAME_BYTES)
        require(offset == previous and size > 0 and offset+size <= start, 'archive entries do not cover payload in order')
        entries.append({'record_index': index, 'name': name, 'name_encoding': encoding,
                        'file_offset': offset, 'bytes': size, 'index_hex': row.hex()})
        previous = offset + size
    require(previous == start, 'unindexed named archive payload')
    return start, entries


def ground_prefix(data):
    width, height = struct.unpack_from('<II', data)
    layer_count = data[8]
    grid_offset = 9 + layer_count * 6
    require(grid_offset + 4 <= len(data), 'truncated terrain layers')
    layers = [dict(zip(('resource','x','y'), struct.unpack_from('<HHH', data, 9+i*6))) for i in range(layer_count)]
    grid_width, grid_height = struct.unpack_from('<HH', data, grid_offset)
    require(width and height and 0 < grid_width <= GROUND_GRID_LIMIT and 0 < grid_height <= GROUND_GRID_LIMIT, 'invalid terrain dimensions')
    end = grid_offset + 4 + grid_width * grid_height
    require(end <= len(data), 'truncated terrain grid')
    return {'width':width,'height':height,'layers':layers,'grid_width':grid_width,
            'grid_height':grid_height,'cell_order':'x-major','cells_hex':data[grid_offset+4:end].hex(), 'bytes_read':end}



def ground_record(data):
    """Complete native FUN_004121a8 layout; unknown semantics stay explicit."""
    terrain = ground_prefix(data)
    position = terrain['bytes_read']

    def read(layout):
        nonlocal position
        size = struct.calcsize(layout)
        require(position + size <= len(data), 'truncated terrain tail')
        result = struct.unpack_from(layout, data, position)
        position += size
        return result

    count, = read('<H')
    terrain['unknown_triples'] = [dict(zip(('unknown_u16_0', 'unknown_u16_1', 'unknown_u16_2'), read('<HHH'))) for _ in range(count)]
    count, = read('<H')
    terrain['objects'] = [dict(zip(('resource', 'x', 'y'), read('<IHH'))) for _ in range(count)]
    flag0, flag1, count = read('<BBB')
    terrain['unknown_flags'] = [flag0, flag1]
    terrain['unknown_records'] = [dict(zip(('unknown_u32_0', 'unknown_u16_4', 'unknown_u16_6', 'unknown_u8_8'), read('<IHHB'))) for _ in range(count)]
    terrain['unknown_block_hex'] = bytes(read('<10B')).hex()
    terrain['unknown_u32_pair'] = list(read('<II'))
    terrain['color_rgb'] = list(read('<BBB'))
    if position < len(data):
        flag, rows, columns = read('<BBB')
        fields = ('unknown_u16_0', 'unknown_u16_2', 'unknown_u8_4', 'unknown_u8_5', 'unknown_u8_6', 'unknown_u32_7')
        terrain['optional_grid'] = {'unknown_flag': flag, 'rows': rows, 'columns': columns,
            'cell_order': 'row-major', 'cells': [dict(zip(fields, read('<HHBBBI'))) for _ in range(rows * columns)]}
    require(position == len(data), 'unparsed terrain tail')
    terrain['bytes_read'] = position
    return terrain

def export_named(data, ground=False):
    start, entries = named_index(data)
    for row in entries:
        record = data[row['file_offset']:row['file_offset']+row['bytes']]
        row['decoded_hex'] = record.hex()
        if ground:
            row['terrain'] = ground_record(record)
        else:
            require(len(record) in (21,22), 'unsupported WEM record length')
            row['fields'] = {'resource_id':u32(record), 'unknown_u32_offset_4':u32(record,4),
                'unknown_u32_offset_8':u32(record,8), 'unknown_u8_offset_12':record[12],
                **{f'unknown_u16_offset_{offset}':u16(record,offset) for offset in (13,15,17,19)},
                **({'unknown_u8_offset_21':record[21]} if len(record)==22 else {})}
    return {'format':'plaintext-named-archive','decoder_reference':'aLogin FUN_0010f154; Ground FUN_004121a8',
            'index_offset': start,'footer_count_hex':data[-INDEX_COUNT_BYTES:].hex(),'entries':entries,
            'interpretation':'All record bytes parsed and retained; unresolved field meanings use explicit unknown names.'}


def export_audio(source, output, metadata, copy_payload_file=True):
    with source.open('rb') as stream, mmap.mmap(stream.fileno(), 0, access=mmap.ACCESS_READ) as data:
        start, entries = named_index(data)
        total_pages = 0
        for row in entries:
            position, end = row['file_offset'], row['file_offset']+row['bytes']
            pages = 0
            warnings = []
            serial = None
            while position < end:
                header = data[position:position+OGG_HEADER_BYTES]
                require(len(header) == OGG_HEADER_BYTES and header[:4] == b'OggS' and header[4] == 0, 'invalid Ogg page header')
                current_serial, sequence = struct.unpack_from('<II', header, 14)
                if pages == 0:
                    require(header[5] & OGG_BEGIN_STREAM and sequence == 0, 'invalid Ogg beginning')
                    serial = current_serial
                require(current_serial == serial and sequence == pages, 'Ogg serial or page sequence changed')
                segments = header[26]
                page_size = OGG_HEADER_BYTES + segments + sum(data[position+OGG_HEADER_BYTES:position+OGG_HEADER_BYTES+segments])
                require(position+page_size <= end, 'Ogg page exceeds indexed stream')
                position += page_size
                if position == end:
                    if not header[5] & OGG_END_STREAM:
                        warnings.append('source stream lacks the Ogg end-of-stream flag')
                else:
                    require(not header[5] & OGG_END_STREAM, 'unexpected Ogg end-of-stream')
                pages += 1
            row['pages'] = pages
            if warnings:
                row['source_warnings'] = warnings
            total_pages += pages
        footer_hex = data[start:].hex()
        payload_hash = hashlib.sha256(memoryview(data)[:start]).hexdigest()
    if copy_payload_file:
        copy_payload(source, output, start)
        require(sha256(output) == payload_hash, 'audio payload copy checksum mismatch')
    result = dict(metadata, format='plaintext-indexed-ogg-audio', payload_file=output.name,
                  payload_bytes=start, payload_sha256=payload_hash, pages=total_pages,
                  entries=entries, footer_hex=footer_hex,
                  interpretation='payload_file contains consecutive indexed Ogg streams. Extract an entry by its file_offset and bytes. Append footer_hex to reconstruct the original archive.')
    if not copy_payload_file:
        result.pop('payload_file')
        result['track_directory'] = source.stem.lower()
        result['interpretation'] = 'Individual Ogg tracks live in track_directory. Concatenate entries in index order and append footer_hex to reconstruct the original archive.'
    return result


def export_animation(data):
    count = u16(data, len(data)-INDEX_COUNT_BYTES)
    start = len(data)-INDEX_COUNT_BYTES-count*ANIMATION_INDEX_BYTES
    require(count and start >= 0, 'invalid animation index')
    rows = []
    previous = 0
    for index in range(count):
        position = start + index*ANIMATION_INDEX_BYTES
        row = data[position:position+ANIMATION_INDEX_BYTES]
        identifier, offset, size = struct.unpack('<HII', row)
        require(offset == previous and size >= ANIMATION_MIN_RECORD_BYTES and offset+size <= start, 'invalid animation record boundary')
        rows.append({'record_index':index,'id':identifier,'file_offset':offset,'bytes':size,
                     'index_hex':row.hex(),'decoded_hex':data[offset:offset+size].hex()})
        previous = offset+size
    require(previous == start, 'unindexed animation bytes')
    return {'format':'plaintext-animation-archive','index_offset':start,
            'footer_count_hex':data[-INDEX_COUNT_BYTES:].hex(),'records':rows,
            'interpretation':'Full plaintext animation records retained; timing and movement semantics are partially understood.'}


def export_eve(data):
    require(len(data) >= EVE_HEADER_BYTES, 'truncated EVE header')
    count = u32(data, 8)
    table_end = EVE_HEADER_BYTES + count * EVE_INDEX_BYTES
    require(table_end <= len(data), 'invalid EVE index')
    rows = []
    covered = []
    for index in range(count):
        position = EVE_HEADER_BYTES + index * EVE_INDEX_BYTES
        native_index = data[position:position+EVE_INDEX_BYTES]
        identifier, scene, start, size = struct.unpack('<HHIH', native_index)
        require(start >= table_end and size >= EVE_SECTION_COUNT*4 and start+size <= len(data), 'invalid EVE map bounds')
        block = data[start:start+size]
        directory = size-EVE_SECTION_COUNT*4
        offsets = struct.unpack_from('<'+'I'*EVE_SECTION_COUNT, block, directory)
        sections = []
        for category, offset in enumerate(offsets):
            if offset:
                end = min([directory]+[value for value in offsets if value > offset])
                require(offset < end <= directory, 'invalid EVE section bounds')
                sections.append({'category':category, 'name':EVE_SECTION_NAMES[category],
                    'offset':offset,'bytes':end-offset,'entry_count':u16(block,offset)})
        rows.append({'record_index':index,'id':identifier,'scene':scene,'file_offset':start,
                     'index_hex':native_index.hex(),'sections':sections,'decoded_hex':block.hex()})
        covered.append((start,start+size))
    # Preserve gaps, padding and orphan blocks in addition to indexed maps.
    gaps = []
    previous = table_end
    for start,end in sorted(covered):
        require(start >= previous, 'overlapping EVE maps')
        if start > previous:
            gaps.append({'file_offset':previous,'decoded_hex':data[previous:start].hex()})
        previous=end
    if previous < len(data):
        gaps.append({'file_offset':previous,'decoded_hex':data[previous:].hex()})
    return {'format':'plaintext-eve','header_hex':data[:EVE_HEADER_BYTES].hex(), 'maps':rows,'gaps':gaps,
            'interpretation':'All map, event, NPC, battle and section bytes retained; section directory decoded without applying gameplay patches.'}


def export_formula(data):
    expected = 1+FORMULA_COEFFICIENT_COUNT*8+4+FORMULA_GATE_COUNT*2
    require(len(data) == expected and data[0] == FORMULA_VERSION, 'unsupported Formula.dat layout')
    return {'format':'plaintext-formula','version':data[0],
            'coefficients':list(struct.unpack_from('<'+'d'*FORMULA_COEFFICIENT_COUNT,data,1)),
            'level_scaler':u32(data,1+FORMULA_COEFFICIENT_COUNT*8),
            'level_gates':list(struct.unpack_from('<'+'H'*FORMULA_GATE_COUNT,data,1+FORMULA_COEFFICIENT_COUNT*8+4)),
            'decoded_hex':data.hex()}


def export_lbd(data):
    count = u16(data)
    require(2+count <= len(data), 'truncated LBD counts')
    counts=data[2:2+count]
    position=2+count
    rows=[]
    for index, child_count in enumerate(counts):
        require(position+3+child_count*3 <= len(data),'truncated LBD group')
        identifier,flag=struct.unpack_from('<HB',data,position)
        position+=3
        children=[]
        for _ in range(child_count):
            child_id,child_flag=struct.unpack_from('<HB',data,position)
            children.append({'id':child_id,'flags':child_flag})
            position+=3
        rows.append({'record_index':index,'id':identifier,'flags':flag,'children':children})
    require(position == len(data),'unparsed LBD bytes')
    return {'format':'plaintext-lbd','groups':rows,'decoded_hex':data.hex(),
            'decoder_reference':'aLogin FUN_001ada04'}


def export_converted_items(data):
    require(len(data)%CONVERTED_ITEM_BYTES == 0,'invalid converted item size')
    rows=[]
    for offset in range(0,len(data),CONVERTED_ITEM_BYTES):
        record=data[offset:offset+CONVERTED_ITEM_BYTES]
        name,encoding=readable_text(record[1:21])
        rows.append({'record_index':offset//CONVERTED_ITEM_BYTES,'file_offset':offset,
                     'id':u16(record,22),'name':name,'name_encoding':encoding,
                     'type':record[21],'equip_slot':u16(record,28),'level':u16(record,30),
                     'status':[u16(record,35),u16(record,37)],
                     'values':list(struct.unpack_from('<ii',record,39)), 'decoded_hex':record.hex()})
    return {'format':'plaintext-converted-items','record_bytes':CONVERTED_ITEM_BYTES,'records':rows,
            'interpretation':'Server-only converted item table retained separately. Never merged into the authoritative client item_data.json.'}


def xml_node(node):
    return {'tag':node.tag,'attributes':dict(node.attrib),'text':node.text,'tail':node.tail,
            'children':[xml_node(child) for child in node]}


def export_text(data,suffix):
    # Full source bytes preserve BOMs, line endings, comments and text encoding.
    try:
        text=data.decode('utf-8-sig');encoding='utf-8-sig' if data.startswith(b'\xef\xbb\xbf') else 'utf-8'
    except UnicodeDecodeError:
        text,encoding=readable_text(data)
    result={'format':'plaintext-text','encoding':encoding,'text':text,'source_hex':data.hex()}
    if suffix=='.json':
        result['format']='plaintext-json';result['value']=json.loads(text)
    elif suffix=='.csv':
        result['format']='plaintext-csv';result['rows']=list(csv.reader(io.StringIO(text)))
    elif suffix=='.wlo':
        result['format']='plaintext-xml';result['document']=xml_node(ET.fromstring(data))
    return result


CLIENT_OUTPUT_NAMES={
    'adjustridepetpos.txt':'ride_pet_positions.json',
    'playertitledata.txt':'player_titles.json', 'trafficsetting.txt':'traffic_settings.json',
    'ground.mmg':'ground_data.json','wem.mmg':'wem_data.json','eve.emg':'eve_data.json',
    'formula.dat':'formula_data.json','lbd.dat':'lbd_data.json','skilldata.mbtm':'animation_data.json',
}
TEXT_SUFFIXES={'.txt','.csv','.json','.md','.wlo'}
# Original account databases, sidecars and database-path overrides are not assets.
EXCLUDED_SOURCE_FILES={'serverdatabase.db', 'serverdatabase.db-wal',
                         'serverdatabase.db-shm', 'serverdatabase.db-journal',
                       'database.override.txt'}


def inventory(client,server):
    require(client.is_dir() and (server/'Data').is_dir(),'missing client data or server Data directory')
    selected={}
    shadowed={}
    for path in sorted((server/'Data').rglob('*')):
        if path.is_file():
            key=path.relative_to(server/'Data').as_posix().lower()
            if key in EXCLUDED_SOURCE_FILES:
                continue
            require(key not in selected,'case-insensitive server filename collision')
            selected[key]=('server',path)
    for path in sorted(client.rglob('*')):
        if path.is_file():
            key=path.relative_to(client).as_posix().lower()
            if key in EXCLUDED_SOURCE_FILES:
                continue
            if key in selected:
                shadowed[key]=selected[key][1]
            selected[key]=('client',path)
    for path in sorted((server/'listdata').rglob('*')):
        if path.is_file():
            selected['listdata/'+path.relative_to(server/'listdata').as_posix().lower()]=('server-listdata',path)
    return selected,shadowed


def export_all(client,server,destination):
    client=client.resolve();server=server.resolve();destination=destination.resolve()
    require(not destination.is_relative_to(client) and not destination.is_relative_to(server), 'output must be outside source projects')
    selected,shadowed=inventory(client,server)
    # Reject unsupported future assets rather than claim an incomplete export.
    for key,(_,source) in selected.items():
        require(key in FORMATS or key in CLIENT_OUTPUT_NAMES or key in ('item.dat','itemdat.wpdat','odd.dat','odd_d01.dat') or source.suffix.lower() in TEXT_SUFFIXES, 'unsupported data file: '+key)
    manifest={'schema_version':SCHEMA_VERSION,'client_source':str(client),'server_source':str(server),
              'priority':'client-over-server','scope':'Client/data, server/Data and server/listdata resources. Original account databases, SQLite sidecars, database-path overrides, source code, project metadata and licensing documents are excluded.',
              'assets':[]}
    audio_archives=[]
    for key,(origin,source) in sorted(selected.items()):
        source_hash=sha256(source)
        metadata={'schema_version':SCHEMA_VERSION,'source':str(source),'source_bytes':source.stat().st_size,'source_sha256':source_hash}
        payload_path=None
        if key=='item.dat':
            output=destination/'item_data.json'
            subprocess.run(['go','run','./cmd/item-export','-input',str(source),'-output',str(output)],cwd=REPO,check=True,stdout=subprocess.DEVNULL)
            result=json.loads(output.read_text())
            require(result['source_sha256']==source_hash,'item export checksum mismatch')
            status='decrypted';count=len(result['items'])
        elif key in ('odd.dat','odd_d01.dat'):
            stem=source.stem.lower()
            output=destination/'audio'/(stem+'_index.json')
            payload_path=destination/'audio'/(stem+'.ogg')
            result=export_audio(source,payload_path,metadata,copy_payload_file=False)
            from extract_audio import extract_source
            audio_archives.append(extract_source(source,destination/'audio',result))
            write_json(output,result);status='plaintext-audio';count=len(result['entries'])
        else:
            data=source.read_bytes()
            if key in FORMATS:
                spec=FORMATS[key];output=destination/spec['output'];result=export_records(data,spec);status='decrypted'
            elif key=='ground.mmg' or key=='wem.mmg':
                output=destination/CLIENT_OUTPUT_NAMES[key];result=export_named(data,ground=key=='ground.mmg');status='plaintext-archive'
            elif key=='skilldata.mbtm':
                output=destination/CLIENT_OUTPUT_NAMES[key];result=export_animation(data);status='plaintext-archive'
            elif key=='eve.emg':
                output=destination/CLIENT_OUTPUT_NAMES[key];result=export_eve(data);status='plaintext-archive'
            elif key=='formula.dat':
                output=destination/CLIENT_OUTPUT_NAMES[key];result=export_formula(data);status='plaintext-table'
            elif key=='lbd.dat':
                output=destination/CLIENT_OUTPUT_NAMES[key];result=export_lbd(data);status='plaintext-table'
            elif key=='itemdat.wpdat':
                output=destination/'server'/'converted_item_data.json';result=export_converted_items(data);status='plaintext-table'
            else:
                if key in CLIENT_OUTPUT_NAMES:
                    output=destination/CLIENT_OUTPUT_NAMES[key]
                else:
                    relative=Path(key)
                    stem=relative.stem
                    if key=='item_mall.txt':
                        stem+='_text'
                    output=destination/'server'/relative.parent/(stem+'.json')
                result=export_text(data,source.suffix.lower());status='plaintext-text'
            count=len(result.get('records',result.get('entries',result.get('maps',result.get('groups',result.get('tables',[]))))))
            result=dict(metadata,**result)
            write_json(output,result)
        if key == 'skill.dat':
            from skill_effects import annotate
            annotate(output, destination / 'skill_effects.json')
        require(sha256(source)==source_hash,'source changed while exporting: '+key)
        entry={'asset':key,'origin':origin,**metadata,'status':status,'records':count,
               'output':output.relative_to(destination).as_posix(),'output_bytes':output.stat().st_size,'output_sha256':sha256(output)}
        if payload_path:
            entry['audio_tracks']={'directory':(payload_path.parent/payload_path.stem).relative_to(destination).as_posix(),'bytes':result['payload_bytes'],'sha256':result['payload_sha256']}
        if key in shadowed:
            other=shadowed[key];other_hash=sha256(other)
            entry['overridden_server_source']={'source':str(other),'bytes':other.stat().st_size,'sha256':other_hash,'identical':other_hash==source_hash}
        manifest['assets'].append(entry)
        if key == 'skill.dat':
            from skill_effects import entry as effect_entry
            manifest['assets'].append(effect_entry(entry, destination / 'skill_effects.json'))
        print(f'{key}: {status}, {count} records -> {entry["output"]}',flush=True)
    from lucky_draw import generate as generate_lucky_draw, entry as lucky_draw_entry
    item_entry = next(row for row in manifest['assets'] if row['asset'] == 'item.dat')
    lucky_output = destination / 'lucky_draw.json'
    generate_lucky_draw(Path(item_entry['source']), lucky_output)
    manifest['assets'].append(lucky_draw_entry(item_entry, lucky_output))
    if audio_archives:
        write_json(destination/'audio'/'manifest.json',{'schema_version':SCHEMA_VERSION,'archives':audio_archives})
    write_json(destination/'asset_manifest.json',manifest)
    print(f'Exported all {len(selected)} selected assets; client overrides {len(shadowed)} server copies.',flush=True)
    return manifest


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client',type=Path,default=REPO.parent/'Wonderland-Client'/'data')
    parser.add_argument('--server',type=Path,default=REPO.parent/'Wonderland-Private-Server')
    parser.add_argument('--output',type=Path,default=REPO/'data')
    arguments=parser.parse_args()
    export_all(arguments.client,arguments.server,arguments.output)


if __name__=='__main__':
    main()

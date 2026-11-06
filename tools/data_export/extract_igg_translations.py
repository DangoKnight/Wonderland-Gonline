"""Match IGG English text to modern native exports, without replacing raw data.

NPC/Item English layouts reserve 20 name bytes (Chinese: 10/14). Cipher
fields after the name move by 10/6 bytes; the remaining layout is unchanged.
"""
import argparse
from collections import Counter, defaultdict
import csv
import hashlib
import json
from pathlib import Path
import re
import struct

from export import FORMATS, REPO, export_eve, export_records, require, sha256, transform, write_json

IGG_NPC_NAME_EXTRA = 10
IGG_ITEM_NAME_EXTRA = 6
IGG_ITEM_BYTES = 457
IGG_ITEM_VERSION_OFFSET = 120
IGG_ITEM_NAME_BYTES = 20
ITEM_NAME_BYTES = 14
ITEM_DESCRIPTION_LENGTH = 146
ITEM_DESCRIPTION_START = 147
ITEM_DESCRIPTION_BYTES = 254
ITEM_ID_OFFSET = 16
# Independent format table from the Item.dat loader, not values from exports.
ITEM_CIPHER = dict(size=451, prefix=0, bias=9,
    masks={1:0x9a, 2:0xefc3, 4:0x0b80f4b4},
    offsets={1:[15,34,35,44,45,46,47,112,113,122,129,401,402,403,406,407,408,415,416,417,418,419,420],
             2:[16,18,20,22,24,26,28,30,32,123,134,136,138,140,142,144,404,409,411,413,421,423,425,427,429],
             4:[36,40,48,52,56,60,64,68,72,76,80,84,88,92,96,100,104,108,114,118,125,130,431,435,439,443,447]},
    reversals=[(1,15),(147,401)])
CSV_HEADER = ['table','id','field','original','english','match','source_record_sha256','target_record_sha256','source']
TABLES = ('npc', 'item', 'skill', 'scene', 'talk', 'mark')
TABLE_FILES = {'npc':'Npc.dat','item':'Item.dat','skill':'Skill.dat','scene':'SceneData.Dat','talk':'Talk.dat','mark':'Mark.dat'}
EXPORT_FILES = {'npc':'npc_data.json','item':'item_data.json','skill':'skill_data.json','scene':'scene_data.json','talk':'talk_data.json','mark':'mark_data.json'}
EVENT_SECTIONS = (4, 7, 9)  # Events, question groups, pre-events (ParseEVEMap).
ACTION_PLAYER, ACTION_ACTOR = 1, 2
PLAYER_SPEECH, ACTOR_SPEECH, ACTOR_SPEECH_ALTERNATE = 2, 0, 1
EVE_EVENT_NAME_BYTES, EVE_OPERAND_BYTES = 20, 21


def expanded(spec, extra):
    return dict(spec, size=spec['size']+extra,
        offsets={w:[v+extra for v in offsets] for w,offsets in spec['offsets'].items()},
        reversals=[(1,1+20)] + [(a+extra,b+extra) for a,b in spec['reversals'][1:]])


def english_text(raw):
    # IGG is Western text, not Big5. Keep only printable English/ASCII so the
    # legacy font renderer never receives guessed replacement characters.
    raw = raw.rstrip(b'\x00 ')
    return raw.decode('ascii') if all(b in (9,10,13) or 32 <= b < 127 for b in raw) else ''


def decode_igg(table, data):
    if table == 'item':
        require(len(data)%IGG_ITEM_BYTES == 0 and int.from_bytes(data[IGG_ITEM_VERSION_OFFSET:IGG_ITEM_VERSION_OFFSET+4], 'little') == 9, 'unsupported IGG Item.dat layout')
        spec = expanded(ITEM_CIPHER, IGG_ITEM_NAME_EXTRA)
        rows = []
        for offset in range(IGG_ITEM_BYTES, len(data), IGG_ITEM_BYTES):
            wire = data[offset:offset+IGG_ITEM_BYTES]
            decoded = transform(wire, spec)
            require(transform(decoded,spec,encrypt=True) == wire, 'IGG item round trip failed')
            require(decoded[0] <= IGG_ITEM_NAME_BYTES, 'invalid IGG item name length')
            # Canonicalize non-text offsets; full English names remain separate.
            canonical = decoded[:1]+decoded[1:1+ITEM_NAME_BYTES]+decoded[1+IGG_ITEM_NAME_BYTES:]
            desc_len = canonical[ITEM_DESCRIPTION_LENGTH]
            rows.append(dict(fields={'id':int.from_bytes(canonical[ITEM_ID_OFFSET:ITEM_ID_OFFSET+2],'little')},
                name={'text':english_text(decoded[1:1+decoded[0]])},
                description={'text':english_text(canonical[ITEM_DESCRIPTION_START:ITEM_DESCRIPTION_START+desc_len])},
                decoded_hex=canonical.hex(), source_hex=decoded.hex()))
        return rows
    key = TABLE_FILES[table].lower()
    spec = FORMATS[key]
    if table == 'npc':
        spec = dict(expanded(spec,IGG_NPC_NAME_EXTRA), version=(48,4,12),
            texts={'name':(0,1,20)}, names={o+IGG_NPC_NAME_EXTRA:n for o,n in spec['names'].items()})
    rows = export_records(data,spec)['records']
    for row in rows:
        raw = bytes.fromhex(row['decoded_hex'])
        row['source_hex'] = raw.hex()
        for field,(length,start,capacity) in spec['texts'].items():
            require(raw[length] <= capacity, 'invalid IGG text length')
            row[field]['text'] = english_text(raw[start:start+raw[length]])
        if table == 'npc':
            row['decoded_hex'] = (raw[:11]+raw[21:]).hex()
    return rows


def unique_rows(rows):
    result = {}
    for row in rows:
        ident = row['fields']['id']
        require(ident and ident not in result, f'duplicate/zero native ID {ident}')
        result[ident] = row
    return result


def target_rows(table, document):
    if table != 'item':
        return unique_rows(document['records'])
    rows = []
    for row in document['items']:
        rows.append(dict(fields={'id':row['definition']['id']}, name={'text':row['definition']['name']},
            description={'text':row['description']}, decoded_hex=row['decoded_hex']))
    return unique_rows(rows)


def event_talk_contexts(maps):
    """Use actual event/branch/operation identities, never record row positions."""
    result = defaultdict(set)
    for m in maps:
        raw = bytes.fromhex(m['decoded_hex'])
        offsets = struct.unpack_from('<11I',raw,len(raw)-44)
        for section in EVENT_SECTIONS:
            pos = offsets[section]
            if not pos:
                continue
            end = min([len(raw)-44]+[o for o in offsets if o > pos])
            def read(size):
                nonlocal pos
                require(pos+size <= end, 'truncated EVE event section')
                value = raw[pos:pos+size]
                pos += size
                return value
            count = int.from_bytes(read(2),'little')
            for _ in range(count):
                event = int.from_bytes(read(2),'little')
                if section == 7:
                    read(EVE_EVENT_NAME_BYTES+3)
                    for _ in range(read(1)[0]):
                        slot,talk,kind,entry_type = struct.unpack('<BHBB',read(5))
                        if kind == 1 and talk:
                            result[talk].add(('question',m['id'],event,slot,entry_type))
                    continue
                read(1+EVE_EVENT_NAME_BYTES)
                for _ in range(read(1)[0]):
                    branch = read(1)[0]
                    read(EVE_OPERAND_BYTES)
                    for _ in range(read(1)[0]):
                        operation = read(1)[0]
                        data = read(EVE_OPERAND_BYTES)
                        code, d1,d2,d3,d4 = struct.unpack_from('<B4H',data)
                        if code == ACTION_PLAYER and d1 == PLAYER_SPEECH:
                            talk = d2
                        elif code == ACTION_ACTOR and d2 in (ACTOR_SPEECH,ACTOR_SPEECH_ALTERNATE):
                            talk = d3
                        else:
                            continue
                        if talk:
                            result[talk].add((m['id'],section,event,branch,operation,code,d1 if code == ACTION_ACTOR else 0))
    return result


def movie_talk_contexts(styles):
    result = defaultdict(set)
    for name,doc in styles.items():
        stem = Path(name).stem
        movie = str(int(stem)) if stem.isdigit() else stem.lower()
        flat = [v for row in doc.get('extra_records',[]) for v in row]
        count = doc['section_counts'][4]
        for index in range(1,count+1):
            offset = 4*index-2  # TMovie loader's shifted extra-record storage.
            require(offset+4 <= len(flat), 'truncated movie dialogue records')
            speaker,talk,stage,delay = flat[offset:offset+4]
            if talk:
                result[talk].add(('movie',movie,index,speaker,stage))
    return result


def merge_contexts(target,other):
    for ident,contexts in other.items():
        target[ident].update(contexts)


def without_text(table, raw):
    raw = bytearray(raw)
    spans = [(0,15),(146,401)] if table == 'item' else [(length,start+capacity) for length,start,capacity in FORMATS[TABLE_FILES[table].lower()]['texts'].values()]
    for start,end in spans:
        raw[start:end] = bytes(end-start)
    return bytes(raw)


def identity_matches(table, source, target):
    a,b = bytes.fromhex(source['decoded_hex']), bytes.fromhex(target['decoded_hex'])
    if table == 'npc':
        # Type, ID, body sprite, icon, portrait and element. Stat balancing may differ.
        spans = [(11,18),(57,58),(88,90)]
    elif table == 'item':
        # Type, ID, inventory art, large art, body overlays, equipment slot.
        spans = [(15,30),(46,47)]
    elif table == 'skill':
        # ID, type, element/category/layer, target policy and icon.
        spans = [(21,24),(26,30),(99,102)]
    elif table == 'scene':
        spans = [(0,2),(23,25)]  # Scene identity and environment category.
    else:
        return without_text(table,a) == without_text(table,b)
    return all(a[start:end] == b[start:end] for start,end in spans)


def build_overrides(table, source, target, contexts=None, authored_names=frozenset()):
    accepted, report = [], []
    fields = ('name','description') if table in ('item','skill','mark') else (('text',) if table == 'talk' else ('name',))
    for ident, row in sorted(source.items()):
        other = target.get(ident)
        if other is None:
            report.append((table,ident,'','missing-modern-id'))
            continue
        if not identity_matches(table,row,other):
            report.append((table,ident,'','identity-mismatch'))
            continue
        for field in fields:
            original, english = other[field]['text'], row[field]['text']
            english = english.strip()
            if field == 'name':
                english = re.sub(r'\s*\(quest\)', '', english, flags=re.I).strip()
                english = ' '.join(english.split())
            if not english or not re.search('[A-Za-z]',english):
                report.append((table,ident,field,'not-readable-English'))
                continue
            # Retain identical IGG names when a fallback patch would change
            # them: these rows explicitly assert English-source priority.
            if english == original and not (field == 'name' and (table,ident) in authored_names):
                report.append((table,ident,field,'already-identical'))
                continue
            basis = 'id+presentation-identity'
            if table == 'talk':
                if not contexts or not contexts[0].get(ident,set()) & contexts[1].get(ident,set()):
                    report.append((table,ident,field,'no-shared-EVE-dialogue-context'))
                    continue
                basis = 'id+unchanged-metadata+shared-EVE-context'
                # Variable markers must stay compatible with the modern interpreter.
                tokens = lambda s: sorted(re.findall(r'%[-+0-9.]*[a-zA-Z]|\{[^{}]*\}|<[^<>]+>',s))
                if tokens(english) != tokens(original):
                    report.append((table,ident,field,'placeholder-mismatch'))
                    continue
            if field == 'description' and without_text(table,bytes.fromhex(row['decoded_hex'])) != without_text(table,bytes.fromhex(other['decoded_hex'])):
                report.append((table,ident,field,'description-gameplay-metadata-changed'))
                continue
            accepted.append([table,ident,field,original,english,basis,
                hashlib.sha256(bytes.fromhex(row['source_hex'])).hexdigest(),
                hashlib.sha256(bytes.fromhex(other['decoded_hex'])).hexdigest()])
    return accepted, report


def extract(igg, target, output, report_dir, fallbacks=()):
    for directory in (target,igg,*fallbacks):
        require(not output.resolve().is_relative_to(directory.resolve()) and
                not report_dir.resolve().is_relative_to(directory.resolve()),
                'translation output must be outside source installations and exports')
    target_context = event_talk_contexts(json.loads((target/'eve_data.json').read_text(encoding='utf-8'))['maps'])
    modern_styles = target/'media/sty/style_data.json'
    if modern_styles.is_file():
        merge_contexts(target_context,movie_talk_contexts(json.loads(modern_styles.read_text(encoding='utf-8'))['styles']))
    authored_names=set()
    with (REPO/'internal/assets/npc_name_reviews.csv').open(encoding='utf-8',newline='') as f:
        authored_names.update(('npc',int(row['id'])) for row in csv.DictReader(f) if row['certain'] == 'true')
    for table,filename in (('npc','npc_names.csv'),('item','item_names.csv')):
        with (REPO/'internal/assets'/filename).open(encoding='utf-8',newline='') as f:
            authored_names.update((table,int(row['id'])) for row in csv.DictReader(f))
    rows, report, sources, seen, installations = [], [], [], set(), []
    from styles import decode_style
    for installation in (igg,*fallbacks):
        paths = {p.name.lower():p for p in (installation/'data').iterdir() if p.is_file()}
        source_context = event_talk_contexts(export_eve(paths['eve.emg'].read_bytes())['maps'])
        style_hashes,unsupported_styles = {},[]
        for path in sorted((installation/'sty').glob('*')):
            if path.suffix.lower() != '.sty':
                continue
            try:
                doc=decode_style(path.read_bytes())
                merge_contexts(source_context,movie_talk_contexts({path.name:doc}))
                style_hashes[path.name]=sha256(path)
            except ValueError as error:
                unsupported_styles.append(dict(file=path.name,error=str(error)))
        installations.append(dict(source=installation.name,eve_sha256=sha256(paths['eve.emg']),styles=style_hashes,unsupported_styles=unsupported_styles))
        for table in TABLES:
            path = paths[TABLE_FILES[table].lower()]
            document = json.loads((target/EXPORT_FILES[table]).read_text(encoding='utf-8'))
            english = unique_rows(decode_igg(table,path.read_bytes()))
            modern = target_rows(table,document)
            accepted,rejected = build_overrides(table,english,modern,(source_context,target_context),authored_names)
            # The newer installation owns shared record IDs, even when its
            # English text already equals modern text or a match is rejected.
            accepted=[r for r in accepted if (table,r[1]) not in seen]
            rows.extend([*r,installation.name] for r in accepted)
            seen.update((table,ident) for ident in english)
            report.extend((installation.name,*r) for r in rejected)
            sources.append(dict(source=installation.name,table=table,file=path.name,sha256=sha256(path),records=len(english),modern_records=len(modern),overrides=len(accepted),target_sha256=sha256(target/EXPORT_FILES[table])))
            print(f'{installation.name}/{table}: {len(english)} English records; {len(accepted)} text overrides',flush=True)
    output.parent.mkdir(parents=True,exist_ok=True)
    with output.open('w',encoding='utf-8',newline='') as f:
        writer=csv.writer(f,lineterminator='\n');writer.writerow(CSV_HEADER);writer.writerows(rows)
    report_dir.mkdir(parents=True,exist_ok=True)
    with (report_dir/'igg_translation_review.csv').open('w',encoding='utf-8',newline='') as f:
        writer=csv.writer(f,lineterminator='\n');writer.writerow(['source','table','id','field','reason']);writer.writerows(report)
    manifest=dict(format='igg-english-text-overrides-v1',sources=sources,total_overrides=len(rows),
        decisions=dict(Counter(row[-1] for row in report)),output_sha256=sha256(output),
        installations=installations,target_eve_sha256=sha256(target/'eve_data.json'))
    write_json(report_dir/'igg_translation_sources.json',manifest)
    return manifest


if __name__ == '__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--igg',type=Path,required=True,help='original English IGG installation')
    parser.add_argument('--fallback',type=Path,action='append',default=[],help='older English installation, used only for missing record IDs')
    parser.add_argument('--target',type=Path,default=REPO/'data',help='modern lossless asset exports')
    parser.add_argument('--output',type=Path,default=REPO/'internal/clienttext/igg_overrides.csv')
    parser.add_argument('--report-dir',type=Path,default=REPO/'var/reports')
    args=parser.parse_args()
    extract(args.igg,args.target,args.output,args.report_dir,args.fallback)

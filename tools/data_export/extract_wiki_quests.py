#!/usr/bin/env python3
"""Inventory saved public MediaWiki API responses without copying article prose."""
import argparse
import base64
import collections
import csv
import hashlib
import html
import json
from pathlib import Path
import re

from extract_monster_names import write_csv

WIKI = 'https://wonderlandonline.fandom.com'
QUEST_MARK_CONDITION = 5
QUEST_MARK_ACTION = 5
PLAYER_ACTION = 1
PLAYER_REWARD = 1
ITEM_REWARD = 1
GOLD_REWARD = 2
EXPERIENCE_REWARD = 5
ADVANCE_MARK = 1
COMPLETE_MARK = 2
ACTOR_ACTION = 2
ACTOR_PRESENTATION_MODES = (4, 8, 10)  # Animation, expression, authored turn.
BATTLE_ACTION = 4
BATTLE_SLOT_COUNT = 8
MONSTER_TEMPLATE_MINIMUM = 10000


def native_word(raw, offset):
    return raw[offset] | raw[offset + 1] << 8


def actor_candidates(catalog, rows, historical_rules, modes=ACTOR_PRESENTATION_MODES, identity_prefix='wiki_actor'):
    events = {tuple(map(int, entry.split(':'))) for row in rows for entry in row['native_events'].split(';') if entry}
    policies, evidence = [], []
    for mid, eid in sorted(events):
        for event in catalog['maps'][str(mid)].get('events') or []:
            if event['ClickID'] != eid:
                continue
            for branch in event['Branches']:
                for operation in branch.get('Operations') or []:
                    raw = operation['Data']
                    if raw[0] != ACTOR_ACTION or native_word(raw, 3) not in modes:
                        continue
                    pattern = dict(code=ACTOR_ACTION, d1=native_word(raw, 1), d2=native_word(raw, 3), d3=native_word(raw, 5), value=int.from_bytes(bytes(raw[8:12]), 'little'))
                    covered = False
                    for rule in historical_rules:
                        if rule['kind'] != 'actor_capability':
                            continue
                        scope = rule.get('scope', {})
                        if any(scope.get(k) and v not in scope[k] for k, v in [('maps', mid), ('events', eid), ('branches', branch['Index'])]):
                            continue
                        if all(pattern.get(k) == v for k, v in rule.get('pattern', {}).items()):
                            covered = True
                            break
                    # Exclude historical capabilities even if an operator removed
                    # one later; a new batch must not silently restore that policy.
                    if covered:
                        continue
                    if pattern['d3'] > 255:
                        raise ValueError('actor presentation operand exceeds native byte')
                    identity = f'{identity_prefix}_{mid}_{eid}_{branch["Index"]}_{operation["Index"]}'
                    policies.append(dict(id=identity, kind='actor_capability', scope=dict(maps=[mid], events=[eid], branches=[branch['Index']], allow_missing=True), pattern=pattern))
                    evidence.append(dict(policy_id=identity, map=mid, event=eid, branch=branch['Index'], operation=operation['Index'], mode=pattern['d2'], actor=pattern['d1'], operand=pattern['d3']))
    if len({p['id'] for p in policies}) != len(policies):
        raise ValueError('duplicate actor policy identity')
    return policies, evidence


def battle_formations(catalog, rows):
    references = {tuple(map(int, entry.split(':'))) for row in rows for entry in row['native_events'].split(';') if entry}
    formations, actions = set(), 0
    for mid, eid in references:
        for event in catalog['maps'][str(mid)].get('events') or []:
            if event['ClickID'] != eid:
                continue
            for branch in event['Branches']:
                for operation in branch.get('Operations') or []:
                    raw = operation['Data']
                    if raw[0] == BATTLE_ACTION:
                        actions += 1
                        formations.add((mid, native_word(raw, 3)))
    result = []
    for mid, identity in sorted(formations):
        native = next((f for f in catalog['maps'][str(mid)].get('battles') or [] if f['ClickID'] == identity), None)
        if not native:
            raise ValueError('missing native quest battle formation')
        raw = native.get('Team1') or []
        raw = base64.b64decode(raw, validate=True) if isinstance(raw, str) else raw
        if len(raw) % 3:
            raise ValueError('incomplete native battle member record')
        members = [(native_word(raw, i), raw[i+2]) for i in range(0, len(raw), 3) if native_word(raw, i) >= MONSTER_TEMPLATE_MINIMUM]
        if not members or any(not 1 <= slot <= BATTLE_SLOT_COUNT for _, slot in members):
            raise ValueError('unresolved native battle slot')
        if any(str(template) not in catalog['npcs'] for template, _ in members):
            raise ValueError('missing quest enemy definition')
        result.append(dict(map=mid, entry=identity, members=';'.join(f'{template}@{slot}' for template, slot in members),
                           initial_occupants=len({slot for _, slot in members}), total_occupants=len(members),
                           flat_list_overspawns=len({slot for _, slot in members[:BATTLE_SLOT_COUNT]}) < min(len(members), BATTLE_SLOT_COUNT)))
    return result, actions


def completion_candidates(catalog, rows, aliases, extended=False):
    allowed_marks = {int(mark) for row in rows for mark in row['native_marks'].split(';') if mark}
    names = collections.defaultdict(set)
    for row in rows:
        for mark in row['native_marks'].split(';'):
            if mark:
                names[int(mark)].add(f'{row["notepad_page"]}:{row["slot"]}')
    def supported(raw):
        # Additional opt-in bundles preserve companion/skill rewards and their
        # native receipts in the same resource transaction.
        if extended and raw[0] == 3:
            mode, identity, compatibility = native_word(raw, 1), native_word(raw, 3), native_word(raw, 5)
            amount = int.from_bytes(bytes(raw[8:12]), 'little')
            return str(identity) in catalog['npcs'] and ((mode == 1 and compatibility == 1) or (mode == 2 and compatibility == 0)) and amount == 0
        if extended and raw[0] == 11:
            return str(native_word(raw, 1)) in catalog['skills'] and native_word(raw, 3) == 1 and native_word(raw, 5) == 0 and int.from_bytes(bytes(raw[8:12]), 'little') == 1
        if raw[0] == QUEST_MARK_ACTION:
            return native_word(raw, 1) != 0 and native_word(raw, 3) in (ADVANCE_MARK, COMPLETE_MARK)
        if raw[0] != PLAYER_ACTION or native_word(raw, 1) != PLAYER_REWARD:
            return False
        kind, identity = native_word(raw, 3), native_word(raw, 5)
        return kind == ITEM_REWARD and str(identity) in catalog['items'] or kind == GOLD_REWARD and identity == 0 or kind == EXPERIENCE_REWARD and identity == 1
    rules, evidence = [], []
    for mid, definition in catalog['maps'].items():
        for event in definition.get('events') or []:
            for branch in event.get('Branches') or []:
                if branch['Condition'][0] != QUEST_MARK_CONDITION:
                    continue
                operations = branch.get('Operations') or []
                index = len(operations)
                while index and supported(operations[index - 1]['Data']):
                    index -= 1
                tail = operations[index:]
                marks = {native_word(o['Data'], 1) for o in tail if o['Data'][0] == QUEST_MARK_ACTION and native_word(o['Data'], 3) == COMPLETE_MARK} & allowed_marks
                if not marks or not any(o['Data'][0] == PLAYER_ACTION or extended and o['Data'][0] in (3, 11) for o in tail):
                    continue
                key = (int(mid), event['ClickID'], branch['Index'])
                identity = aliases.get(key, f'wiki_native_completion_{key[0]}_{key[1]}_{key[2]}')
                raw = tail[0]['Data']
                value = (raw[8] | int.from_bytes(bytes(raw[9:13]), 'little') << 8) & 0xffffffff
                rule = dict(id=identity, kind='quest_completion', scope=dict(maps=[key[0]], events=[key[1]], branches=[key[2]], allow_missing=True), value=index, mark=min(marks),
                            pattern=dict(code=raw[0], d1=native_word(raw, 1), d2=native_word(raw, 3), d3=native_word(raw, 5), value=value))
                rules.append(rule)
                evidence.append(dict(policy_id=identity, map=key[0], event=key[1], branch=key[2], tail_index=index, completion_mark=min(marks),
                                     notepad_positions=';'.join(sorted(set().union(*(names[m] for m in marks)))), method='exact native resource/mark tail; quest-gated entry branch'))
    priority = {identity: i for i, identity in enumerate(aliases.values())}
    rules.sort(key=lambda r: (priority.get(r['id'], len(priority)), r['scope']['maps'][0], r['scope']['events'][0], r['scope']['branches'][0]))
    evidence.sort(key=lambda r: (priority.get(r['policy_id'], len(priority)), r['map'], r['event'], r['branch']))
    return rules, evidence


def reward_bundle_candidates(catalog, rows, aliases):
    base, _ = completion_candidates(catalog, rows, aliases)
    original = {(r['scope']['maps'][0], r['scope']['events'][0], r['scope']['branches'][0]): r for r in base}
    extended, evidence = completion_candidates(catalog, rows, {}, extended=True)
    policies, parents, proof = [], {}, []
    for rule, row in zip(extended, evidence):
        key = (rule['scope']['maps'][0], rule['scope']['events'][0], rule['scope']['branches'][0])
        old = original.get(key)
        if old and old['value'] == rule['value']:
            continue
        rule['id'] = f'wiki_quest_bundle_{key[0]}_{key[1]}_{key[2]}'
        rule['kind'] = 'quest_reward_bundle'
        row['policy_id'] = rule['id']
        row['method'] = 'exact native resource/companion/skill/mark tail; quest-gated entry branch'
        if old:
            parents[rule['id']] = old['id']
        policies.append(rule)
        proof.append(row)
    return policies, parents, proof


def text(value):
    return ' '.join(html.unescape(re.sub('<[^>]+>', ' ', value)).split())


def normalize(value):
    return re.sub('[^a-z0-9]', '', re.sub(r'^(the|a)\s+', '', value.lower()))


def notepad_index(response):
    body = response['parse']['text']
    body = body['*'] if isinstance(body, dict) else body
    rows = []
    for table in re.findall(r'<table\b[^>]*>(.*?)</table>', body, re.S):
        page = re.search(r'<th\b[^>]*>\s*Page (\d+)', table)
        if not page:
            continue
        slot = 0
        for cell in re.findall(r'<td\b[^>]*>(.*?)</td>', table, re.S):
            name = text(cell)
            if not name:
                continue
            slot += 1
            link = re.search(r'<a\b[^>]*href="([^"]+)"[^>]*>', cell)
            title = re.search(r'title="([^"]+)"', link[0]) if link else None
            rows.append(dict(notepad_page=int(page[1]), slot=slot, name=name,
                             wiki_title=html.unescape(title[1]) if title else name,
                             index_link=WIKI + html.unescape(link[1]) if link else ''))
    expected = re.search(r'(\d+) Quests Total', text(body))
    if not rows or expected and len(rows) != int(expected[1]):
        raise ValueError('incomplete notepad index')
    keys = [(r['notepad_page'], r['slot']) for r in rows]
    if len(set(keys)) != len(keys):
        raise ValueError('duplicate notepad position')
    return rows


def write_notepad_document(path, rows):
    lines = ['# Wiki notepad quest list', '',
             f'This lists all {len(rows)} entries in the [source notepad index]({WIKI}/wiki/Quests_by_Notepad_Page), preserving its page order and labels.', '',
             'Native mark bindings identify existing scripts; they do not certify complete playthroughs. See [research and implementation status](QUEST_RESEARCH.md).', '']
    for page in sorted({r['notepad_page'] for r in rows}):
        lines.extend([f'## Notepad page {page}', '', '| Slot | Quest | Native marks |', '| --- | --- | --- |'])
        for row in rows:
            if row['notepad_page'] != page:
                continue
            name = row['name'].replace('|', '\\|').replace('[', '\\[').replace(']', '\\]')
            url = row['source_url'].replace('(', '%28').replace(')', '%29')
            lines.append(f'| {row["slot"]} | [{name}]({url}) | {row["native_marks"].replace(";", ", ")} |')
        lines.append('')
    path.write_text('\n'.join(lines), encoding='utf-8', newline='\n')


def event_references(catalog):
    result = collections.defaultdict(set)
    for mid, definition in catalog['maps'].items():
        for event in definition.get('events') or []:
            marks = set()
            for branch in event.get('Branches') or []:
                condition = branch['Condition']
                if condition[0] == QUEST_MARK_CONDITION:
                    marks.add(condition[1] | condition[2] << 8)
                for operation in branch.get('Operations') or []:
                    raw = operation['Data']
                    if raw[0] == QUEST_MARK_ACTION:
                        marks.add(raw[1] | raw[2] << 8)
            for mark in marks:
                result[mark].add((int(mid), event['ClickID']))
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cache', type=Path, required=True)
    p.add_argument('--native-catalog', type=Path, required=True)
    p.add_argument('--output-dir', type=Path, required=True)
    p.add_argument('--notepad-document', type=Path)
    p.add_argument('--historical-story', type=Path, default=Path(__file__).resolve().parents[2] / 'internal/assets/story_defaults.json')
    p.add_argument('--completion-aliases', type=Path, default=Path(__file__).parent / 'reference/fandom-quests/completion_policy_aliases.csv')
    p.add_argument('--notepad-aliases', type=Path, default=Path(__file__).parent / 'reference/fandom-quests/notepad_aliases.csv')
    a = p.parse_args()
    index = a.cache / 'notepad-1.txt'
    rows = notepad_index(json.loads(index.read_text(encoding='utf-8')))
    pages, aliases = {}, {}
    source_files = [index, a.cache / 'category.json']
    for path in sorted(a.cache.glob('batch-*.json')):
        source_files.append(path)
        query = json.loads(path.read_text(encoding='utf-8'))['query']
        for alias in query.get('normalized', []) + query.get('redirects', []):
            aliases[alias['from']] = alias['to']
        for page in query.get('pages', []):
            pages[page['title']] = page
    category = json.loads((a.cache / 'category.json').read_text(encoding='utf-8'))
    if 'continue' in category:
        raise ValueError('incomplete quest category')
    members = {r['title'] for r in category['query']['categorymembers']}
    catalog = json.loads(a.native_catalog.read_text(encoding='utf-8'))
    native = collections.defaultdict(set)
    for identity, q in catalog['quests'].items():
        native[normalize(q['title'])].add(int(identity))
    refs = event_references(catalog)
    with a.notepad_aliases.open(encoding='utf-8', newline='') as stream:
        alias_rows = list(csv.DictReader(stream))
    bindings = {(int(r['notepad_page']), int(r['slot'])): r for r in alias_rows}
    if len(bindings) != len(alias_rows):
        raise ValueError('duplicate notepad alias identity')
    for row in rows:
        canonical, seen = row['wiki_title'], set()
        while canonical in aliases and canonical not in seen:
            seen.add(canonical)
            canonical = aliases[canonical]
        page = pages.get(canonical, {})
        revision = page.get('revisions', [{}])[0]
        marks = native[normalize(row['name'])] | native[normalize(row['wiki_title'])] | native[normalize(canonical)]
        binding = bindings.get((row['notepad_page'], row['slot']))
        method, notes = 'exact title candidate', ''
        if binding:
            if binding['source_name'] != row['name']:
                raise ValueError('notepad source identity changed')
            marks = {int(i) for i in binding['native_marks'].split(';')}
            if any(str(i) not in catalog['quests'] or catalog['quests'][str(i)]['title'] != binding['expected_native_title'] for i in marks):
                raise ValueError('native alias identity changed')
            method, notes = 'authored alias/script binding', binding['evidence']
        events = sorted(set().union(*(refs[mark] for mark in marks))) if marks else []
        row.update(canonical_title=canonical, article_available=bool(page and not page.get('missing')), revision_id=revision.get('revid', ''),
                   native_marks=';'.join(map(str, sorted(marks))), native_events=';'.join(f'{mid}:{eid}' for mid, eid in events),
                   disabled_events=';'.join(f'{mid}:{eid}' for mid, eid in events if str(mid << 16 | eid) in catalog['disabled']),
                   implementation_status='native-script-unverified' if events else 'native-identity-unresolved',
                   binding_method=method, binding_notes=notes,
                   source_url=WIKI + '/wiki/' + canonical.replace(' ', '_'))
    write_csv(a.output_dir / 'notepad_quests.csv', rows, list(rows[0]))
    if a.notepad_document:
        write_notepad_document(a.notepad_document, rows)
    category_rows = [dict(title=title, source_url=WIKI + '/wiki/' + title.replace(' ', '_')) for title in sorted(members)]
    write_csv(a.output_dir / 'category_pages.csv', category_rows, ['title', 'source_url'])
    with a.completion_aliases.open(encoding='utf-8', newline='') as stream:
        aliases = {(int(r['map']), int(r['event']), int(r['branch'])): r['policy_id'] for r in csv.DictReader(stream)}
    rules, evidence = completion_candidates(catalog, rows, aliases)
    write_csv(a.output_dir / 'completion_candidates.csv', evidence, list(evidence[0]))
    (a.output_dir / 'wiki_quest_defaults.json').write_text(json.dumps(rules, indent=2) + '\n', encoding='utf-8', newline='\n')
    bundles, bundle_parents, bundle_evidence = reward_bundle_candidates(catalog, rows, aliases)
    write_csv(a.output_dir / 'reward_bundle_candidates.csv', bundle_evidence, ['policy_id', 'map', 'event', 'branch', 'tail_index', 'completion_mark', 'notepad_positions', 'method'])
    (a.output_dir / 'wiki_quest_bundle_candidates.json').write_text(json.dumps(bundles, indent=2) + '\n', encoding='utf-8', newline='\n')
    (a.output_dir / 'wiki_quest_bundle_parents.json').write_text(json.dumps(bundle_parents, indent=2) + '\n', encoding='utf-8', newline='\n')
    actor_rules, actor_evidence = actor_candidates(catalog, rows, json.loads(a.historical_story.read_text(encoding='utf-8'))['rules'])
    write_csv(a.output_dir / 'actor_candidates.csv', actor_evidence, ['policy_id', 'map', 'event', 'branch', 'operation', 'mode', 'actor', 'operand'])
    (a.output_dir / 'wiki_quest_actor_candidates.json').write_text(json.dumps(actor_rules, indent=2) + '\n', encoding='utf-8', newline='\n')
    star_rules, star_evidence = actor_candidates(catalog, rows, json.loads(a.historical_story.read_text(encoding='utf-8'))['rules'], modes=(11,), identity_prefix='wiki_constellation_actor')
    write_csv(a.output_dir / 'constellation_actor_candidates.csv', star_evidence, ['policy_id', 'map', 'event', 'branch', 'operation', 'mode', 'actor', 'operand'])
    (a.output_dir / 'wiki_quest_constellation_actor_candidates.json').write_text(json.dumps(star_rules, indent=2) + '\n', encoding='utf-8', newline='\n')
    formations, battle_actions = battle_formations(catalog, rows)
    write_csv(a.output_dir / 'battle_formations.csv', formations, ['map', 'entry', 'members', 'initial_occupants', 'total_occupants', 'flat_list_overspawns'])
    manifest = dict(index_url=WIKI + '/wiki/Quests_by_Notepad_Page', category_url=WIKI + '/wiki/Category:Quests',
                    notepad_quests=len(rows), category_articles=len(members), fetched_canonical_articles=sum(not p.get('missing', False) for p in pages.values()),
                    linked_native_scripts=sum(bool(r['native_events']) for r in rows),
                    completion_candidates=len(rules), completion_checkpoints=len({r['mark'] for r in rules}),
                    reward_bundle_candidates=len(bundles), extended_existing_completions=len(bundle_parents),
                    actor_candidates=len(actor_rules), historical_story_sha256=hashlib.sha256(a.historical_story.read_bytes()).hexdigest(),
                    constellation_actor_candidates=len(star_rules),
                    native_battle_actions=battle_actions, native_battle_formations=len(formations), formations_with_premature_flat_spawns=sum(r['flat_list_overspawns'] for r in formations),
                    completion_aliases_sha256=hashlib.sha256(a.completion_aliases.read_bytes()).hexdigest(),
                    notepad_aliases_sha256=hashlib.sha256(a.notepad_aliases.read_bytes()).hexdigest(),
                    sources=[dict(file=f.name, sha256=hashlib.sha256(f.read_bytes()).hexdigest()) for f in source_files],
                    native_catalog_sha256=hashlib.sha256(a.native_catalog.read_bytes()).hexdigest())
    (a.output_dir / 'sources.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({k: v for k, v in manifest.items() if k not in ('sources', 'native_catalog_sha256')}))


if __name__ == '__main__':
    main()

"""Export authored Lucky Draw policy against freshly decoded sibling Item.dat."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile
from export import REPO, inventory, sha256, write_json, require

ASSET = 'lucky_draw.json'
RULES = REPO / 'internal/assets/lucky_draw_rules.json'

def generate(source, destination):
    source_hash = sha256(source)
    policy = json.loads(RULES.read_text())
    with tempfile.TemporaryDirectory(prefix='wonderland-lucky-draw-') as temporary:
        items = Path(temporary) / 'items.json'
        subprocess.run(['go', 'run', './cmd/item-export', '-input', str(source),
                        '-output', str(items)], cwd=REPO, check=True, stdout=subprocess.DEVNULL)
        decoded = json.loads(items.read_text())
    require(decoded['source_sha256'] == source_hash == sha256(source), 'Lucky Draw item source changed')
    definitions = {row['definition']['id']: row['definition'] for row in decoded['items']}
    rewards = []
    slots = set()
    for row in policy['rewards']:
        require(row['item_id'] in definitions, f"unknown Lucky Draw item {row['item_id']}")
        require(type(row['weight']) is int and row['weight'] > 0 and
                type(row['quantity']) is int and 1 <= row['quantity'] <= 255 and
                type(row['slot']) is int and 1 <= row['slot'] <= 255 and row['slot'] not in slots,
                'invalid Lucky Draw policy')
        slots.add(row['slot'])
        rewards.append({**row, 'name': definitions[row['item_id']]['name']})
    require(rewards and sum(row['weight'] for row in rewards) <= (1 << 63) - 1, 'invalid Lucky Draw total weight')
    write_json(destination, {'schema_version': 1, 'format': 'lucky-draw-rewards',
                            'source': str(source), 'source_sha256': source_hash,
                            'source_bytes': source.stat().st_size, 'reference': policy['reference'],
                            'reference_date': policy['reference_date'], 'notes': policy['notes'],
                            'policy_sha256': sha256(RULES), 'value': rewards})

def entry(item_entry, destination):
    return {**{key: item_entry[key] for key in ('origin', 'source', 'source_bytes', 'source_sha256')},
            'asset': ASSET, 'status': 'derived-compatibility-rewards', 'records': len(json.loads(destination.read_text())['value']),
            'output': destination.name, 'output_bytes': destination.stat().st_size,
            'output_sha256': sha256(destination)}

def refresh(destination):
    manifest_path = destination / 'asset_manifest.json'
    manifest = json.loads(manifest_path.read_text())
    # Decode sibling input afresh; existing exported item data is never an input.
    selected, _ = inventory(REPO.parent / 'Wonderland-Client/data',
                            REPO.parent / 'Wonderland-Private-Server')
    _, source = selected['item.dat']
    output = destination / ASSET
    generate(source, output)
    item_entry = next(row for row in manifest['assets'] if row['asset'] == 'item.dat')
    require(sha256(source) == item_entry['source_sha256'], 're-export items after source update')
    manifest['assets'] = [row for row in manifest['assets'] if row['asset'] != ASSET] + [entry(item_entry, output)]
    write_json(manifest_path, manifest)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--overwrite', action='store_true')
    args = parser.parse_args()
    if not args.overwrite:
        parser.error('--overwrite is required to replace authored reward exports')
    refresh(args.output)

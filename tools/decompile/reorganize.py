#!/usr/bin/env python3
"""Reorganize the reference aLogin decompile into per-unit files.

The reference decompile (Wonderland-Private-Server/decompiled/aLogin_decompiled.c)
is the source of truth, but it holds code only, and none of the available
executables is the build it came from. This tool aligns it with a decompile of
an available build (same program, different link layout), then borrows that
build's metadata: unit boundaries, class VMTs and published method names.

Inputs:
  REF.c     reference decompile
  OTHER.c   decompile of OTHER.exe (Ghidra, same script format)
  OTHER.exe the executable OTHER.c was made from
  META      output of delphi_meta.py for OTHER.exe
Outputs, under OUT:
  index.json       one record per reference function
  units/NNN_*.c    reference functions grouped by unit, annotated
  map.json         reference address -> OTHER address

Functions of OTHER with no counterpart in REF (mostly methods reached only
through VMTs, which Ghidra missed in the reference) are added to their unit
files as supplements, flagged "supplement", with the reference address
predicted from the nearest matched function.

Usage: reorganize.py REF.c OTHER.c OTHER.exe META OUT
"""
import bisect
import collections
import hashlib
import json
import os
import re
import sys

from delphi_meta import Image

HEADER = re.compile(r'// Function: (\S+) @ ([0-9a-f]+)')
NORMALIZE = [
    (re.compile(r'(FUN|LAB|DAT|PTR_DAT|PTR_PTR|PTR_FUN|PTR_LAB|UNK|switchD|caseD|PTR|s)_[0-9a-f]{5,8}(_[0-9a-f]+)?'), 'X'),
    (re.compile(r'0x[0-9a-f]{5,8}'), 'A'),
    (re.compile(r'func_0x[0-9a-f]+'), 'F'),
]
STRING = re.compile(r'"((?:[^"\\]|\\.){3,})"')


def functions(path):
    out, cur, buf = [], None, []
    with open(path, encoding='utf-8', errors='replace') as f:
        for line in f:
            m = HEADER.match(line)
            if m:
                if cur:
                    out.append((cur[0], cur[1], ''.join(buf)))
                cur, buf = (m.group(1), int(m.group(2), 16)), []
            elif cur:
                buf.append(line)
    if cur:
        out.append((cur[0], cur[1], ''.join(buf)))
    return out


def body(text):
    lines = []
    for line in text.splitlines():
        s = line.strip()
        if s and not s.startswith(('//', '/*')):
            lines.append(s)
    return '\n'.join(lines)


def fingerprint(text):
    t = body(text)
    for r, s in NORMALIZE:
        t = r.sub(s, t)
    return hashlib.sha1(t.encode()).hexdigest()


def align(ref, other):
    """Pairs functions by unique normalized bodies kept in address order, then
    fills equal-sized gaps between anchors positionally."""
    hr, ho = [fingerprint(f[2]) for f in ref], [fingerprint(f[2]) for f in other]
    cr, co = collections.Counter(hr), collections.Counter(ho)
    where = {h: j for j, h in enumerate(ho) if co[h] == 1}
    anchors = [(i, where[h]) for i, h in enumerate(hr) if cr[h] == 1 and h in where]
    # Longest chain increasing in both indices.
    tails, tail_idx, prev = [], [], [None] * len(anchors)
    for k, (_, j) in enumerate(anchors):
        p = bisect.bisect_left(tails, j)
        if p == len(tails):
            tails.append(j)
            tail_idx.append(k)
        else:
            tails[p], tail_idx[p] = j, k
        prev[k] = tail_idx[p - 1] if p else None
    chain, k = [], tail_idx[-1] if tail_idx else None
    while k is not None:
        chain.append(anchors[k])
        k = prev[k]
    chain.reverse()
    pairs, exact = dict(chain), set(i for i, _ in chain)
    last = (-1, -1)
    for i, j in chain + [(len(ref), len(other))]:
        if i - last[0] == j - last[1]:
            for d in range(1, i - last[0]):
                pairs[last[0] + d] = last[1] + d
        last = (i, j)
    return pairs, exact


def unit_ranges(img):
    """Unit ends: each unit's initialization/finalization procedures close its code."""
    entry = img.base + img.pe.OPTIONAL_HEADER.AddressOfEntryPoint
    code = img.bytes(entry, 32)
    i = code.find(b'\xb8')  # mov eax, InitTable
    table = int.from_bytes(code[i + 1:i + 5], 'little')
    n, tab = img.u32(table), img.u32(table + 4)
    ends = sorted({max(img.u32(tab + 8 * k), img.u32(tab + 8 * k + 4)) for k in range(n)} - {0})
    return ends


def main(ref_path, other_path, exe, meta, out):
    ref, other = functions(ref_path), functions(other_path)
    pairs, exact = align(ref, other)
    img = Image(exe)
    classes = json.load(open(os.path.join(meta, 'classes.json')))
    slots, published = {}, {}
    for c in classes:
        for k, addr in enumerate(c['virtuals']):
            slots.setdefault(addr, []).append('%s.vmt+0x%x' % (c['name'], 4 * k))
        for name, addr in c['published'].items():
            published[addr] = '%s.%s' % (c['name'], name)
    vmt_owner = {c['vmt']: c['name'] for c in classes}
    ends = unit_ranges(img)

    def unit(addr):
        return bisect.bisect_left(ends, addr)

    # Label each unit range by the classes whose VMTs fall in it.
    labels = collections.defaultdict(list)
    for va, name in vmt_owner.items():
        labels[unit(va)].append(name)

    index, by_unit, mapping = [], collections.defaultdict(list), {}
    for i, (name, addr, text) in enumerate(ref):
        rec = {'address': '%08x' % addr, 'name': name}
        if i in pairs:
            o = other[pairs[i]][1]
            mapping['%x' % addr] = '%x' % o
            rec['other'] = '%08x' % o
            rec['match'] = 'exact' if i in exact else 'positional'
            rec['unit'] = unit(o)
            if o in slots:
                rec['virtual'] = slots[o]
            if o in published:
                rec['published'] = published[o]
        strings = sorted(set(STRING.findall(text)))
        if strings:
            rec['strings'] = strings[:40]
        index.append(rec)
        by_unit[rec.get('unit', -1)].append((rec, text))

    # Supplements: OTHER functions without a reference counterpart.
    matched_other = set(pairs.values())
    anchors = sorted((other[j][1], ref[i][1]) for i, j in pairs.items())
    anchor_other = [a[0] for a in anchors]
    for j, (name, addr, text) in enumerate(other):
        if j in matched_other:
            continue
        k = bisect.bisect_right(anchor_other, addr) - 1
        predicted = anchors[k][1] + (addr - anchors[k][0]) if k >= 0 else None
        rec = {'address': '%08x' % predicted if predicted is not None else '', 'name': name,
               'other': '%08x' % addr, 'match': 'supplement', 'unit': unit(addr)}
        if addr in slots:
            rec['virtual'] = slots[addr]
        if addr in published:
            rec['published'] = published[addr]
        index.append(rec)
        by_unit[rec['unit']].append((rec, text))

    os.makedirs(os.path.join(out, 'units'), exist_ok=True)
    for u, items in sorted(by_unit.items()):
        label = '_'.join(sorted(labels.get(u, []))[:3]) or 'unit'
        fname = 'unmatched.c' if u < 0 else '%03d_%s.c' % (u, label[:80])
        with open(os.path.join(out, 'units', fname), 'w') as f:
            if u >= 0 and labels.get(u):
                f.write('// Classes: %s\n\n' % ', '.join(sorted(labels[u])))
            for rec, text in sorted(items, key=lambda x: x[0]['address'] or x[0].get('other', '')):
                notes = [k + '=' + (', '.join(v) if isinstance(v, list) else str(v)) for k, v in rec.items()
                         if k in ('other', 'match', 'virtual', 'published')]
                if rec.get('match') == 'supplement':
                    f.write('// Supplement: %s @ %s from the other build; absent from the reference decompile\n' % (rec['name'], rec['other']))
                else:
                    f.write('// Function: %s @ %s\n' % (rec['name'], rec['address']))
                if notes:
                    f.write('// %s\n' % '; '.join(notes))
                f.write(text.rstrip('\n') + '\n\n')
    json.dump(index, open(os.path.join(out, 'index.json'), 'w'), indent=0)
    json.dump(mapping, open(os.path.join(out, 'map.json'), 'w'))
    print('reference', len(ref), 'mapped', len(pairs), 'exact', len(exact), 'supplements', len(other) - len(matched_other), 'units', len(by_unit))


if __name__ == '__main__':
    main(*sys.argv[1:6])

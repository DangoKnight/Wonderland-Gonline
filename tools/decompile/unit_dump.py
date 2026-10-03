#!/usr/bin/env python3
"""Dump the functions of the unit containing an address, without Delphi
exception-frame noise, annotated with class slots and published names.

Usage: unit_dump.py FULL.c aLogin.exe META ADDR [OUT]
"""
import bisect
import json
import os
import re
import sys

from delphi_meta import Image
from reorganize import functions, unit_ranges

# Declarations, blank lines, and the bookkeeping Ghidra shows for Delphi's
# exception frames and pushed return addresses. Calls that take stack
# temporaries as arguments are kept.
NOISE = re.compile(r'in_FS_OFFSET|^\s*$'
                   r'|^\s*(undefined\d?|int|uint|char|byte|bool|short|ushort|float|double|code)\s*\**\s*[a-zA-Z_]\w*( \[\d+\])?;$'
                   r'|^\s*\w*Stack_\w+ = (\([^)]*\))?(0x[0-9a-f]+|&LAB_\w+|&stack0x[0-9a-f]+|&\w*Stack_\w+|\w*Stack_\w+ [+-] (1|-1)|\(undefined\d? \*\*?\)&stack0x[0-9a-f]+);$')


def main(full, exe, meta, addr, out=None):
    addr = int(addr, 16)
    img = Image(exe)
    ends = unit_ranges(img)
    k = bisect.bisect_left(ends, addr)
    lo, hi = (ends[k - 1] if k else 0), ends[k]
    classes = json.load(open(os.path.join(meta, 'classes.json')))
    slots, published = {}, {}
    for c in classes:
        for i, a in enumerate(c['virtuals']):
            if lo < a <= hi:
                slots.setdefault(a, []).append('%s+0x%x' % (c['name'], 4 * i))
        for name, a in c['published'].items():
            published[a] = '%s.%s' % (c['name'], name)
    w = open(out, 'w') if out else sys.stdout
    for name, a, text in functions(full):
        if not (lo < a <= hi):
            continue
        notes = []
        if a in slots:
            own = [s for s in slots[a]]
            notes.append('slots ' + ', '.join(own[:6]) + (' ...' if len(own) > 6 else ''))
        if a in published:
            notes.append(published[a])
        w.write('// %s @ %x%s\n' % (name, a, ('  ; ' + '; '.join(notes)) if notes else ''))
        for line in text.splitlines():
            if not NOISE.search(line) and not line.startswith('// ---'):
                w.write(line + '\n')
        w.write('\n')


if __name__ == '__main__':
    main(*sys.argv[1:])

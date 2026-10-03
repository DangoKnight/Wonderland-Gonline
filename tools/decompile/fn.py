#!/usr/bin/env python3
"""Print functions from a decompile by address: fn.py FILE.c ADDR [ADDR...]"""
import re
import sys

path, wanted = sys.argv[1], {a.lower().lstrip('0') for a in sys.argv[2:]}
show = False
with open(path, encoding='utf-8', errors='replace') as f:
    for line in f:
        m = re.match(r'// (?:Function|Supplement): \S+ @ ([0-9a-f]+)', line)
        if m:
            show = m.group(1).lstrip('0') in wanted
        if show and line.strip() and not line.startswith('// ---'):
            sys.stdout.write(line)

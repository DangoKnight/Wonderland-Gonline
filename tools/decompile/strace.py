#!/usr/bin/env python3
"""List string constants and calls in address order for a code range, from
objdump disassembly. Useful where Ghidra cannot decompile a function.

Usage: strace.py aLogin.exe START END
"""
import re
import subprocess
import sys

from delphi_meta import Image

exe, start, end = sys.argv[1], int(sys.argv[2], 16), int(sys.argv[3], 16)
img = Image(exe)
out = subprocess.run(['objdump', '-d', '-M', 'intel', '--start-address=%#x' % start, '--stop-address=%#x' % end, exe],
                     capture_output=True, text=True).stdout
for line in out.splitlines():
    m = re.match(r'\s*([0-9a-f]+):\s+(?:[0-9a-f]{2} )+\s*(.*)', line)
    if not m:
        continue
    addr, ins = int(m.group(1), 16), m.group(2)
    if ins.startswith('call'):
        print('%x  %s' % (addr, ins))
        continue
    for v in re.findall(r'0x([0-9a-f]{6,8})', ins):
        va = int(v, 16)
        if not img.ok(va, 4) or not img.ok(va - 8, 8):
            continue
        # Delphi AnsiString literal: refcount -1, length, then bytes.
        if img.u32(va - 8) == 0xffffffff and 0 < img.u32(va - 4) < 300:
            s = img.bytes(va, img.u32(va - 4))
            print('%x  %s  ; "%s"' % (addr, ins, s.decode('latin-1')))

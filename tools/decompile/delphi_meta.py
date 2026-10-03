#!/usr/bin/env python3
"""Extract Delphi 7 metadata from aLogin.exe.

Writes, into the output directory:
  units.txt     compiled units, from the PACKAGEINFO resource
  forms/*.txt   form designs (DFM resources) as Delphi text
  classes.json  every VMT: name, parent, instance size, virtual methods,
                published methods (name -> address) and published fields

Usage: delphi_meta.py aLogin.exe OUTDIR
Addresses are virtual addresses (Ghidra uses the PE image base, 0x10000).
"""
import json
import os
import struct
import sys

import pefile

# Delphi 7 VMT offsets, relative to the VMT pointer.
VMT_SELF, VMT_FIELDS, VMT_METHODS, VMT_NAME, VMT_SIZE, VMT_PARENT = -76, -56, -52, -44, -40, -36


class Image:
    def __init__(self, path):
        self.pe = pefile.PE(path)
        self.base = self.pe.OPTIONAL_HEADER.ImageBase
        self.mem = self.pe.get_memory_mapped_image()

    def ok(self, va, n=1):
        rva = va - self.base
        return 0 <= rva and rva + n <= len(self.mem)

    def u8(self, va):
        return self.mem[va - self.base]

    def u16(self, va):
        return struct.unpack_from('<H', self.mem, va - self.base)[0]

    def u32(self, va):
        return struct.unpack_from('<I', self.mem, va - self.base)[0]

    def bytes(self, va, n):
        return self.mem[va - self.base:va - self.base + n]

    def shortstr(self, va):
        n = self.u8(va)
        return self.bytes(va + 1, n).decode('latin-1')

    def section(self, name):
        for s in self.pe.sections:
            if s.Name.rstrip(b'\0').decode() == name:
                return self.base + s.VirtualAddress, s.Misc_VirtualSize
        raise KeyError(name)


def scan_vmts(img):
    code, size = img.section('CODE')
    vmts = []
    for va in range(code + 76, code + size - 4, 4):
        if img.u32(va + VMT_SELF) != va:
            continue
        name_ptr = img.u32(va + VMT_NAME)
        if not img.ok(name_ptr, 2):
            continue
        n = img.u8(name_ptr)
        name = img.bytes(name_ptr + 1, n)
        if not n or not all(32 < c < 127 for c in name):
            continue
        vmts.append(va)
    classes = {}
    starts = sorted(vmts)
    for i, va in enumerate(starts):
        name = img.shortstr(img.u32(va + VMT_NAME))
        parent_ref = img.u32(va + VMT_PARENT)
        parent = img.u32(parent_ref) if parent_ref and img.ok(parent_ref, 4) else 0
        # Virtual methods run from the VMT pointer to the next class's metadata.
        limit = starts[i + 1] + VMT_SELF if i + 1 < len(starts) else va + 4 * 64
        virtuals = []
        p = va
        while p < limit and p < img.u32(va + VMT_NAME):
            target = img.u32(p)
            if not (code <= target < code + size):
                break
            virtuals.append(target)
            p += 4
        classes[va] = {
            'name': name, 'vmt': va, 'parent_vmt': parent, 'instance_size': img.u32(va + VMT_SIZE),
            'virtuals': virtuals, 'published': published_methods(img, va), 'fields': published_fields(img, va),
        }
    for c in classes.values():
        c['parent'] = classes[c['parent_vmt']]['name'] if c['parent_vmt'] in classes else None
    return classes


def published_methods(img, vmt):
    table = img.u32(vmt + VMT_METHODS)
    if not table or not img.ok(table, 2):
        return {}
    out, p = {}, table + 2
    for _ in range(img.u16(table)):
        size, addr = img.u16(p), img.u32(p + 2)
        out[img.shortstr(p + 6)] = addr
        p += size
    return out


def published_fields(img, vmt):
    table = img.u32(vmt + VMT_FIELDS)
    if not table or not img.ok(table, 6):
        return []
    out, p = [], table + 6
    for _ in range(img.u16(table)):
        offset, name = img.u32(p), img.shortstr(p + 6)
        out.append({'offset': offset, 'name': name})
        p += 7 + len(name)
    return out


def resources(pe):
    out = {}
    for t in pe.DIRECTORY_ENTRY_RESOURCE.entries:
        if t.struct.Id != 10:  # RT_RCDATA
            continue
        for e in t.directory.entries:
            name = str(e.name) if e.name else str(e.id)
            leaf = e.directory.entries[0].data.struct
            out[name] = pe.get_data(leaf.OffsetToData, leaf.Size)
    return out


class DFM:
    """Binary form (TPF0) to Delphi text."""

    def __init__(self, data):
        self.d, self.p, self.out = data, 4, []

    def u8(self):
        v = self.d[self.p]
        self.p += 1
        return v

    def take(self, fmt):
        v = struct.unpack_from(fmt, self.d, self.p)
        self.p += struct.calcsize(fmt)
        return v[0]

    def sstr(self):
        n = self.u8()
        s = self.d[self.p:self.p + n].decode('latin-1')
        self.p += n
        return s

    def value(self):
        t = self.u8()
        if t == 0:
            return None
        if t == 1:  # list
            items = []
            while self.d[self.p] != 0:
                items.append(self.value())
            self.p += 1
            return '(' + ' '.join(items) + ')'
        if t == 2:
            return str(self.take('<b'))
        if t == 3:
            return str(self.take('<h'))
        if t == 4:
            return str(self.take('<i'))
        if t == 5:
            raw = self.d[self.p:self.p + 10]
            self.p += 10
            return 'ext:' + raw.hex()
        if t in (6, 7):
            return repr(self.sstr()) if t == 6 else self.sstr()
        if t in (8, 9, 13):  # false, true, nil
            return {8: 'False', 9: 'True', 13: 'nil'}[t]
        if t == 10:  # binary
            n = self.take('<I')
            blob = self.d[self.p:self.p + n]
            self.p += n
            return '{binary %d bytes}' % n if n > 64 else '{' + blob.hex() + '}'
        if t == 11:  # set
            items = []
            while True:
                s = self.sstr()
                if not s:
                    break
                items.append(s)
            return '[' + ', '.join(items) + ']'
        if t == 12:
            n = self.take('<I')
            s = self.d[self.p:self.p + n].decode('latin-1')
            self.p += n
            return repr(s)
        if t == 14:  # collection
            items = []
            while self.d[self.p] != 0:
                if self.d[self.p] in (2, 3, 4):
                    self.value()
                items.append('item ' + ' '.join(self.props()) + ' end')
            self.p += 1
            return '<' + ' '.join(items) + '>'
        if t == 15:
            return str(self.take('<f'))
        if t == 18:
            n = self.take('<I')
            s = self.d[self.p:self.p + 2 * n].decode('utf-16-le')
            self.p += 2 * n
            return repr(s)
        if t == 19:
            return str(self.take('<q'))
        raise ValueError('DFM value type %d at %d' % (t, self.p))

    def props(self):
        out = []
        while self.d[self.p] != 0:
            name = self.sstr()
            out.append('%s = %s' % (name, self.value()))
        self.p += 1
        return out

    def component(self, depth):
        flags = 0
        if self.d[self.p] & 0xf0 == 0xf0:
            flags = self.u8()
            if flags & 2:
                self.value()
        cls, name = self.sstr(), self.sstr()
        pad = '  ' * depth
        self.out.append('%sobject %s: %s' % (pad, name, cls))
        for prop in self.props():
            self.out.append(pad + '  ' + prop)
        while self.d[self.p] != 0:
            self.component(depth + 1)
        self.p += 1
        self.out.append(pad + 'end')

    def text(self):
        self.component(0)
        return '\n'.join(self.out) + '\n'


def package_units(data):
    # PACKAGEINFO: flags, requires count + names, then contains count + (flags, name).
    p = 4
    requires = struct.unpack_from('<I', data, p)[0]
    p += 4
    for _ in range(requires):
        p += 1  # hash
        p = data.index(b'\0', p) + 1
    contains = struct.unpack_from('<I', data, p)[0]
    p += 4
    units = []
    for _ in range(contains):
        p += 2  # flags, hash
        end = data.index(b'\0', p)
        units.append(data[p:end].decode('latin-1'))
        p = end + 1
    return units


def main(exe, out):
    img = Image(exe)
    os.makedirs(os.path.join(out, 'forms'), exist_ok=True)
    res = resources(img.pe)
    with open(os.path.join(out, 'units.txt'), 'w') as f:
        f.write('\n'.join(package_units(res['PACKAGEINFO'])) + '\n')
    for name, data in res.items():
        if data[:4] == b'TPF0':
            with open(os.path.join(out, 'forms', name + '.txt'), 'w') as f:
                f.write(DFM(data).text())
    classes = scan_vmts(img)
    with open(os.path.join(out, 'classes.json'), 'w') as f:
        json.dump(sorted(classes.values(), key=lambda c: c['vmt']), f, indent=1)
    print('units', len(package_units(res['PACKAGEINFO'])), 'forms', sum(d[:4] == b'TPF0' for d in res.values()), 'classes', len(classes))


if __name__ == '__main__':
    main(*sys.argv[1:3])

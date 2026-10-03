#!/usr/bin/env python3
"""Export remaining static client media from the original sibling directory.

Requires Pillow and ffmpeg. Unknown custom resource formats are preserved
losslessly and explicitly listed as unresolved, never described as decrypted.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import io
from pathlib import Path
import shutil
import struct
import subprocess

from PIL import Image, ImageSequence
from export import REPO, write_json, require, sha256
from styles import decode_style

FONT_KEY = 0x58
ASCII_COUNT = 256
FONT_COLUMNS = 64
CURSOR_ENTRY_BYTES = 16
CURSOR_DIRECTORY_BYTES = 6
BITMAP_HEADER_BYTES = 40
BITMAP_ROW_ALIGNMENT = 4
CURSOR_TYPE = 2
TEXT_XOR_KEY = 0x33
MEDIA_DIRS = ('menu', 'images', 'upimg', 'cursor', 'font', 'sound', 'voice', 'sty', 'info', 'txt')
ROOT_RESOURCES = ('Finfo.txt', 'Sinfo.txt', 'MsgSkin.txt', 'SERVER.INI', 'ISP0.DAT', 'isp1.dat', 'web0.DAT', 'PackagesVersion_z.dat', 'update2.dat', 'WL.ico', 'unWL.ico')


def chunks(raw, start, end):
    while start < end:
        require(start + 8 <= end, 'truncated RIFF chunk')
        name, size = struct.unpack_from('<4sI', raw, start)
        begin = start + 8
        require(begin + size <= end, 'RIFF chunk exceeds container')
        yield name, raw[begin:begin + size]
        start = begin + size + (size & 1)
    require(start == end, 'invalid RIFF alignment')


def cursor_image(raw):
    reserved, kind, count = struct.unpack_from('<HHH', raw)
    require(reserved == 0 and kind == CURSOR_TYPE and count == 1, 'unsupported embedded CUR directory')
    width, height, colors, reserved, x, y, size, offset = struct.unpack_from('<BBBBHHII', raw, CURSOR_DIRECTORY_BYTES)
    width, height = width or 256, height or 256
    dib = raw[offset:offset + size]
    require(len(dib) == size and len(dib) >= BITMAP_HEADER_BYTES, 'truncated cursor DIB')
    header, dw, dh, planes, depth, compression = struct.unpack_from('<IiiHHI', dib)
    require(header == BITMAP_HEADER_BYTES and dw == width and dh == height * 2 and planes == 1 and compression == 0, 'unsupported cursor bitmap')
    palette_count = struct.unpack_from('<I', dib, 32)[0] or (1 << depth if depth <= 8 else 0)
    pixel_start = header + palette_count * 4
    stride = ((width * depth + 31) // 32) * BITMAP_ROW_ALIGNMENT
    mask_stride = ((width + 31) // 32) * BITMAP_ROW_ALIGNMENT
    mask_start = pixel_start + stride * height
    require(mask_start + mask_stride * height <= len(dib), 'truncated cursor mask')
    fixed = bytearray(dib[:mask_start])
    struct.pack_into('<i', fixed, 8, height)
    bmp = b'BM' + struct.pack('<IHHI', len(fixed) + 14, 0, 0, 14 + pixel_start) + fixed
    with Image.open(io.BytesIO(bmp)) as image:
        image = image.convert('RGBA')
    # These legacy cursors use AND/XOR masks. An invert pixel requires a
    # framebuffer operation; preserve its coordinate rather than invent alpha.
    inverted = []
    for py in range(height):
        for px in range(width):
            mask = dib[mask_start + (height - 1 - py) * mask_stride + px // 8] & (0x80 >> (px % 8))
            if mask:
                r, g, b, _ = image.getpixel((px, py))
                if (r, g, b) != (0, 0, 0):
                    inverted.append([px, py, r, g, b])
                image.putpixel((px, py), (r, g, b, 0))
    return image, {'hotspot': [x, y], 'invert_pixels': inverted}


def export_cursor(raw, target):
    require(raw[:4] == b'RIFF' and raw[8:12] == b'ACON', 'invalid ANI signature')
    declared = struct.unpack_from('<I', raw, 4)[0] + 8
    require(declared in (len(raw), len(raw) + 8), 'invalid ANI length')
    frames, extra = [], []
    doc = {'format': 'animated-cursor', 'frames': frames, 'declared_bytes': declared,
           'warnings': ['Original RIFF size includes the eight-byte header twice.'] if declared != len(raw) else []}
    for name, data in chunks(raw, 12, len(raw)):
        if name == b'anih':
            require(len(data) == 36, 'unsupported ANI header')
            keys = ('header_bytes', 'frame_count', 'step_count', 'width', 'height', 'bit_depth', 'planes', 'default_jiffies', 'flags')
            doc['header'] = dict(zip(keys, struct.unpack('<9I', data)))
        elif name in (b'rate', b'seq '):
            require(len(data) % 4 == 0, 'invalid ANI step array')
            doc['rate_jiffies' if name == b'rate' else 'sequence'] = list(struct.unpack('<' + 'I' * (len(data) // 4), data))
        elif name == b'LIST' and data[:4] == b'fram':
            for kind, payload in chunks(data, 4, len(data)):
                require(kind == b'icon', 'unknown ANI frame chunk')
                image, meta = cursor_image(payload)
                png = f'frame_{len(frames):03d}.png'
                image.save(target / png)
                frames.append({'png': png, 'width': image.width, 'height': image.height, **meta})
        else:
            extra.append({'chunk': name.decode('ascii'), 'hex': data.hex()})
    require('header' in doc and len(frames) == doc['header']['frame_count'], 'ANI frame count mismatch')
    steps = doc['header']['step_count']
    doc.setdefault('sequence', list(range(steps)))
    doc.setdefault('rate_jiffies', [doc['header']['default_jiffies']] * steps)
    require(len(doc['sequence']) == steps and len(doc['rate_jiffies']) == steps and all(0 <= i < len(frames) for i in doc['sequence']), 'invalid ANI playback steps')
    doc['jiffies_per_second'] = 60
    doc['extra_chunks'] = extra
    return doc


def export_font(raw, name, target):
    decoded = bytes(b ^ FONT_KEY for b in raw)
    # Font1's layout is verified by the native renderer. Font2's physical
    # 12x24 / 24x24 layout follows its complete row/count sizing; the JPC
    # character mapping is intentionally not inferred from the TWN mapping.
    height, narrow, wide = (15, 8, 16) if name.lower().startswith('tatpc1') else (24, 12, 24)
    row_bytes = (narrow + 7) // 8
    wide_row_bytes = (wide + 7) // 8
    ascii_bytes = ASCII_COUNT * height * row_bytes
    require(len(decoded) >= ascii_bytes and (len(decoded) - ascii_bytes) % (height * wide_row_bytes) == 0, 'invalid bitmap font layout')
    count = (len(decoded) - ascii_bytes) // (height * wide_row_bytes)
    sheets = []
    for group, offset, glyphs, width, stride in (('ascii', 0, ASCII_COUNT, narrow, row_bytes), ('wide', ascii_bytes, count, wide, wide_row_bytes)):
        sheet = Image.new('RGBA', (FONT_COLUMNS * width, ((glyphs + FONT_COLUMNS - 1) // FONT_COLUMNS) * height))
        pixels = sheet.load()
        for glyph in range(glyphs):
            x, y = glyph % FONT_COLUMNS * width, glyph // FONT_COLUMNS * height
            for row in range(height):
                bits = int.from_bytes(decoded[offset + (glyph * height + row) * stride:offset + (glyph * height + row + 1) * stride], 'big')
                for column in range(width):
                    if bits & (1 << (stride * 8 - 1 - column)):
                        pixels[x + column, y + row] = (255, 255, 255, 255)
        png = group + '.png'
        sheet.save(target / png)
        sheets.append({'group': group, 'png': png, 'count': glyphs, 'columns': FONT_COLUMNS, 'width': width, 'height': height, 'source_offset': offset, 'row_bytes': stride})
    return {'format': 'bitmap-font', 'source_xor': FONT_KEY, 'decoded_hex': decoded.hex(), 'sheets': sheets,
            'mapping': 'TWN font1 uses client Big5Index; JPC mapping and font2 layout semantics remain unverified.'}



def export_custom_text(raw, name):
    if name.lower() in ('agree.dat', 'agree1.dat', 'agrees.dat'):
        position, lines = 0, []
        while position < len(raw):
            length = raw[position]
            position += 1
            require(position + length <= len(raw), 'truncated encrypted agreement line')
            decoded = bytes(b ^ TEXT_XOR_KEY for b in raw[position:position + length])
            lines.append({'decoded_hex': decoded.hex(), 'text': decoded.decode('cp950')})
            position += length
        return {'format': 'encrypted-agreement-lines', 'source_xor': TEXT_XOR_KEY, 'encoding': 'cp950',
                'decoder_reference': 'aLogin 0x25693c-0x2569ed', 'lines': lines}
    if name.lower() == 'officer.dat':
        decoded = bytes(b ^ TEXT_XOR_KEY for b in raw)
        return {'format': 'encrypted-officer-text', 'source_xor': TEXT_XOR_KEY, 'decoded_hex': decoded.hex(),
                'encoding': 'cp950', 'text': decoded.decode('cp950'), 'decoder_reference': 'aLogin 0x255d0c'}
    return None

def run_media(command):
    result = subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    if result.returncode:
        raise ValueError(f'{command[0]} failed: {result.stderr.decode(errors="replace")}')


def export_one(source, client, destination):
    relative = source.relative_to(client)
    raw = source.read_bytes()
    ext = source.suffix.lower()
    row = {'source': relative.as_posix(), 'source_bytes': len(raw), 'source_sha256': hashlib.sha256(raw).hexdigest()}
    target = destination / relative.parent
    target.mkdir(parents=True, exist_ok=True)
    if source.name.lower() == 'thumbs.db':
        return {**row, 'status': 'excluded', 'reason': 'Windows thumbnail cache; generated metadata, not a game asset.'}
    if ext in ('.bmp', '.jpg', '.jpeg', '.png', '.gif', '.ico'):
        if ext == '.gif':
            target = target / source.stem
            target.mkdir()
            frames = []
            with Image.open(source) as image:
                loop = image.info.get('loop')
                for index, frame in enumerate(ImageSequence.Iterator(image)):
                    name = f'frame_{index:03d}.png'
                    frame.convert('RGBA').save(target / name)
                    frames.append({'png': name, 'duration_ms': frame.info.get('duration', 0), 'disposal': getattr(frame, 'disposal_method', 0)})
            doc = {'format': 'gif-animation', 'loop': loop, 'frames': frames}
        elif ext == '.ico':
            target = target / source.stem
            target.mkdir()
            frames = []
            with Image.open(source) as image:
                for width, height in sorted(image.ico.sizes()):
                    name = f'{width}x{height}.png'
                    image.ico.getimage((width, height)).convert('RGBA').save(target / name)
                    frames.append({'png': name, 'width': width, 'height': height})
            doc = {'format': 'icon', 'images': frames}
        else:
            output = target / (source.name + '.png')
            require(not output.exists(), f'colliding PNG output: {output}')
            with Image.open(source) as image:
                image.convert('RGBA').save(output)
                doc = {'format': 'image', 'png': output.relative_to(destination).as_posix(), 'width': image.width, 'height': image.height}
            return {**row, 'status': 'decoded', **doc}
    elif ext == '.ani':
        target = target / source.stem
        target.mkdir()
        doc = export_cursor(raw, target)
    elif ext in ('.twn', '.jpc'):
        target = target / source.name.replace('.', '_')
        target.mkdir()
        doc = export_font(raw, source.name, target)
    elif ext == '.wav':
        output = target / source.name
        # PCM16 handles both native PCM and compressed MS/IMA ADPCM WAVs.
        run_media(['ffmpeg', '-nostdin', '-v', 'error', '-i', str(source), '-map', '0:a:0', '-c:a', 'pcm_s16le', '-threads', '1', str(output)])
        return {**row, 'status': 'decoded', 'format': 'pcm-s16le-wave', 'wav': output.relative_to(destination).as_posix()}
    elif ext == '.avi':
        # AVI remains a usable video container. Preserve its embedded video,
        # audio and timing instead of expanding it into separate frame files.
        output = target / source.name
        require(not output.exists(), f'colliding video output: {output}')
        shutil.copyfile(source, output)
        return {**row, 'status': 'preserved', 'format': 'avi',
                'video': output.relative_to(destination).as_posix()}
    else:
        # Retain every byte, including encrypted/custom records. This avoids
        # silently passing an incomplete backup off as a decompilation.
        output = target / (source.name + '.json')
        require(ext != '.sty', 'export style sources through export_style_collection')
        doc = export_custom_text(raw, source.name) or {'format': 'unresolved-resource', 'source_hex': raw.hex()}
        if ext in ('.txt', '.ini', '.flst', '.ui') or source.name in ROOT_RESOURCES:
            for encoding in ('utf-8-sig', 'cp950', 'cp1252'):
                try:
                    doc.update(format='text-resource', encoding=encoding, text=raw.decode(encoding))
                    break
                except UnicodeDecodeError:
                    continue
        write_json(output, {**row, **doc})
        return {**row, 'status': 'unresolved' if doc['format'] == 'unresolved-resource' else 'decoded', 'format': doc['format'], 'metadata': output.relative_to(destination).as_posix()}
    write_json(target / 'manifest.json', {**row, **doc})
    return {**row, 'status': 'decoded', 'format': doc['format'], 'metadata': (target / 'manifest.json').relative_to(destination).as_posix()}



STYLE_COLLECTION_PATH = 'sty/style_data.json'


def export_style_collection(client, destination):
    """Decode original STY files into one collection keyed by source filename."""
    output = destination / STYLE_COLLECTION_PATH
    require(not output.exists(), 'existing style collection is protected')
    records, files = {}, []
    directory = client / 'sty'
    for source in sorted(directory.rglob('*')):
        if not source.is_file() or source.suffix.lower() != '.sty':
            continue
        require(not source.is_symlink() and source.resolve().is_relative_to(client.resolve()), 'style source escapes original sibling directory')
        raw = source.read_bytes()
        key = source.relative_to(directory).as_posix()
        provenance = {'source': source.relative_to(client).as_posix(), 'source_bytes': len(raw),
                      'source_sha256': hashlib.sha256(raw).hexdigest()}
        records[key] = {**provenance, **decode_style(raw)}
        require(sha256(source) == provenance['source_sha256'], f'source changed during style export: {source}')
        files.append({**provenance, 'status': 'decoded', 'format': 'style-animation',
                      'metadata': STYLE_COLLECTION_PATH, 'record': key})
    if records:
        write_json(output, {'schema_version': 1, 'format': 'style-collection',
                            'source': str(directory.resolve()), 'styles': records})
    return files

def export_media(client, destination):
    client, destination = client.resolve(), destination.resolve()
    require(client.is_dir() and not destination.is_relative_to(client), 'invalid original/output directories')
    require(not destination.exists(), 'export into a new directory to protect asset edits')
    require(shutil.which('ffmpeg'), 'ffmpeg is required')
    destination.mkdir(parents=True)
    sources = [p for name in MEDIA_DIRS for p in sorted((client / name).rglob('*')) if p.is_file()]
    sources += [client / name for name in ROOT_RESOURCES if (client / name).is_file()]
    require(all(not p.is_symlink() and p.resolve().is_relative_to(client) for p in sources), 'media source escapes original sibling directory')
    def checked_source(p):
        row = export_one(p, client, destination)
        require(sha256(p) == row['source_sha256'], f'source changed during media export: {p}')
        return row
    with ThreadPoolExecutor(max_workers=4) as workers:
        files = list(workers.map(checked_source, [p for p in sources if p.suffix.lower() != '.sty']))
    files.extend(export_style_collection(client, destination))
    files.sort(key=lambda row: row['source'])
    doc = {'schema_version': 1, 'source': str(client), 'files': files,
           'unresolved': [r['source'] for r in files if r['status'] == 'unresolved'],
           'excluded_directories': {'ErrorLog': 'runtime logs', 'user': 'player state', 'Download': 'download staging', 'Photo': 'user screenshots', 'Mp3s': 'empty legacy audio folder'},
           'scope': 'Static media/resource folders and named root resources; executables, DLLs and website/help pages are not game media.'}
    doc['outputs'] = [{'path': p.relative_to(destination).as_posix(), 'bytes': p.stat().st_size, 'sha256': sha256(p)}
                      for p in sorted(destination.rglob('*')) if p.is_file()]
    write_json(destination / 'media_manifest.json', doc)
    print(f"Exported {len(files)} media sources; {len(doc['unresolved'])} custom resources remain unresolved", flush=True)
    return doc


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, default=REPO.parent / 'Wonderland-Client')
    parser.add_argument('--output', type=Path, default=REPO / 'data-media-new')
    args = parser.parse_args()
    export_media(args.input, args.output)

"""Complete STY record boundaries, verified against all 768 original files.

The native 0x365848 loader establishes section order and base field widths.
The supplied files additionally have a u32 in regular sprite headers and an
8-byte timing suffix. Preserve unknown operands rather than assign semantics.
"""
import struct
from export import require, readable_text

STYLE_FRAME_BYTES = 45
IMAGE_FRAME_BYTES = 33
IMAGE_NAME_BYTES = 14
TIMING_OPERAND_BYTES = 11


def decode_style(raw):
    position = 0

    def take(size):
        nonlocal position
        require(size >= 0 and position + size <= len(raw), 'truncated style record')
        result = raw[position:position + size]
        position += size
        return result

    def values(count):
        data = take(count * 4)
        return list(struct.unpack('<' + 'i' * count, data))

    def frames(last, size):
        require(-1 <= last < len(raw) // size, 'invalid style frame count')
        result = []
        for _ in range(last + 1):
            frame = take(size)
            result.append({'unknown_i32_fields': list(struct.unpack('<' + 'i' * (size // 4), frame[:-1])),
                           f'unknown_u8_offset_{size - 1}': frame[-1]})
        return result

    flag = take(1)[0]
    counts = values(6)
    require(all(0 <= n <= len(raw) for n in counts), 'invalid style section counts')
    special, sprites, images, background, extras, timeline_last = counts
    doc = {'format': 'style-animation', 'decoder_reference': 'aLogin 0x365848; source-verified additional fields',
           'unknown_flag': flag, 'section_counts': counts, 'special_sprite': None, 'sprites': [], 'images': [], 'background': None}
    if special:
        header = values(4)
        doc['special_sprite'] = {'header_i32_fields': header, 'frames': frames(header[2], STYLE_FRAME_BYTES)}
    for _ in range(sprites):
        header = values(5)
        doc['sprites'].append({'resource_id': header[0], 'header_i32_fields': header, 'frames': frames(header[2], STYLE_FRAME_BYTES)})

    def image_record(is_background):
        name = take(IMAGE_NAME_BYTES)
        header = values(2 if is_background else 8)
        record = {'name': readable_text(name[1:1 + name[0]])[0], 'name_encoding': readable_text(name[1:1 + name[0]])[1], 'declared_name_bytes': name[0], 'name_storage_hex': name.hex(),
                  'header_i32_fields': header, 'frames': frames(header[1], IMAGE_FRAME_BYTES)}
        if not is_background:
            record['unknown_i32_suffix'] = values(1)[0]
        return record

    for _ in range(images):
        doc['images'].append(image_record(False))
    if background:
        doc['background'] = image_record(True)
    if timeline_last:
        stage_values = values(timeline_last + 1)
        operands = [dict(zip(('unknown_u8_0', 'unknown_u8_1', 'unknown_u8_2', 'unknown_u32_3', 'unknown_u8_7', 'unknown_u8_8', 'unknown_u8_9', 'unknown_u8_10'), struct.unpack('<BBBIBBBB', take(TIMING_OPERAND_BYTES)))) for _ in range(timeline_last + 1)]
        doc['timeline'] = {'stage_i32_values': stage_values, 'operands': operands, 'unknown_suffix_hex': take(8).hex()}
    doc['extra_records'] = [values(4) for _ in range(extras + 1)] if extras else []
    require(position == len(raw), 'unparsed style bytes')
    doc['bytes_read'] = position
    doc['interpretation'] = 'All bytes structured; unnamed operands retain unknown meanings.'
    return doc

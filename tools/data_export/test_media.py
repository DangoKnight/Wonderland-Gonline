"""Independent pixel/byte fixtures for remaining client media decoders."""
import io
from pathlib import Path
import struct
import tempfile
import unittest

from PIL import Image
import media
import export
import styles
from verify_media import reconstruct_style


class MediaTests(unittest.TestCase):
    def test_cursor_hotspot_mask_and_invert_operation(self):
        # Two bottom-up RGB24 pixels: black transparent and white invert.
        dib = struct.pack('<IiiHHIIiiII', 40, 2, 2, 1, 24, 0, 8, 0, 0, 0, 0)
        dib += bytes.fromhex('000000ffffff0000c0000000')
        cur = struct.pack('<HHHBBBBHHII', 0, 2, 1, 2, 1, 0, 0, 1, 0, len(dib), 22) + dib
        image, meta = media.cursor_image(cur)
        self.assertEqual(image.getpixel((0, 0)), (0, 0, 0, 0))
        self.assertEqual(image.getpixel((1, 0)), (255, 255, 255, 0))
        self.assertEqual(meta['hotspot'], [1, 0])
        self.assertEqual(meta['invert_pixels'], [[1, 0, 255, 255, 255]])
        with self.assertRaisesRegex(ValueError, 'truncated'):
            media.cursor_image(cur[:-1])

    def test_agreement_lengths_are_plain_and_content_is_xored(self):
        doc = media.export_custom_text(bytes.fromhex('0342515000'), 'Agree1.dat')
        self.assertEqual([r['text'] for r in doc['lines']], ['qbc', ''])
        reconstructed = b''.join(bytes([len(bytes.fromhex(r['decoded_hex']))]) + bytes(b ^ 0x33 for b in bytes.fromhex(r['decoded_hex'])) for r in doc['lines'])
        self.assertEqual(reconstructed, bytes.fromhex('0342515000'))
        with self.assertRaisesRegex(ValueError, 'truncated'):
            media.export_custom_text(bytes.fromhex('034251'), 'Agree.dat')

    def test_font_ascii_and_wide_bit_order(self):
        plain = bytearray(256 * 15 + 30)
        plain[0], plain[256 * 15], plain[256 * 15 + 1] = 0x81, 0x80, 0x01
        with tempfile.TemporaryDirectory() as temp:
            doc = media.export_font(bytes(b ^ 0x58 for b in plain), 'TATPC1.TWN', Path(temp))
            self.assertEqual(bytes.fromhex(doc['decoded_hex']), bytes(plain))
            with Image.open(Path(temp) / 'ascii.png') as image:
                self.assertEqual([image.getpixel((x, 0))[3] for x in range(8)], [255, 0, 0, 0, 0, 0, 0, 255])
            with Image.open(Path(temp) / 'wide.png') as image:
                self.assertEqual(image.getpixel((0, 0))[3], 255)
                self.assertEqual(image.getpixel((15, 0))[3], 255)
                self.assertEqual(image.getpixel((1, 0))[3], 0)

    def test_style_complete_structures_round_trip(self):
        # Native special sprite, regular sprite, foreground, background,
        # timeline and extra records, with different header widths.
        raw = bytearray(bytes.fromhex('01') + struct.pack('<6i', 1, 1, 1, 1, 1, 1))
        raw += struct.pack('<4i', 77, 1, 0, 0) + bytes(range(45))
        raw += struct.pack('<5i', 78, 2, 0, 1, 255) + bytes(range(45))
        name = bytes.fromhex('03') + b'abc' + bytes(10)
        raw += name + struct.pack('<8i', 1, 0, 2, 3, 4, 5, 6, 7) + bytes(range(33)) + bytes.fromhex('78563412')
        raw += name + struct.pack('<2i', 2, 0) + bytes(range(33))
        raw += struct.pack('<2i', 10, 20) + bytes(range(22)) + bytes(range(8))
        raw += struct.pack('<8i', 1, 2, 3, 4, 5, 6, 7, 8)
        doc = styles.decode_style(raw)
        self.assertEqual(doc['bytes_read'], len(raw))
        self.assertEqual(doc['images'][0]['name'], 'abc')
        self.assertEqual(reconstruct_style(doc), bytes(raw))
        with self.assertRaisesRegex(ValueError, 'truncated'):
            styles.decode_style(raw[:-1])

    def test_ground_complete_tail_and_optional_cells(self):
        wire = struct.pack('<IIBHH', 40, 60, 0, 1, 1) + b'\x07'
        wire += bytes.fromhex('010001000200030001000403020106000800090a00')
        wire += bytes(range(10)) + bytes.fromhex('4433221188776655010203')
        wire += bytes.fromhex('0401010100020003040509080706')
        doc = export.ground_record(wire)
        self.assertEqual(doc['bytes_read'], len(wire))
        self.assertEqual(doc['objects'], [{'resource': 0x01020304, 'x': 6, 'y': 8}])
        self.assertEqual(doc['color_rgb'], [1, 2, 3])
        self.assertEqual(doc['optional_grid']['cells'][0]['unknown_u32_7'], 0x06070809)
        with self.assertRaisesRegex(ValueError, 'truncated'):
            export.ground_record(wire[:-1])


if __name__ == '__main__':
    unittest.main()

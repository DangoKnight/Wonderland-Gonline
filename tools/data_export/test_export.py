"""Wire fixtures and corrupt-input checks for the offline exporters."""
import json
from pathlib import Path
import struct
import tempfile
import unittest

import export
from verify import reconstruct


class DataExportTests(unittest.TestCase):
    def test_native_fixture_width_wrap_and_header(self):
        # Independent wire values: Mark's byte 0x90 -> 249, word 0x2774 ->
        # 65529 and dword 0x082cd1b9 -> 4294967289 after subtracting seven.
        header=bytearray(553);header[258:260]=bytes.fromhex('0400')
        wire=bytearray(553)
        wire[0]=3;wire[252:255]=b'cba'
        wire[255]=0x90
        wire[256:258]=bytes.fromhex('7427')
        wire[533:537]=bytes.fromhex('b9d12c08')
        result=export.export_records(bytes(header+wire),export.FORMATS['mark.dat'])
        first=result['records'][0]
        self.assertEqual(first['name']['text'],'abc')
        self.assertEqual(first['fields']['unknown_u8_offset_255'],249)
        self.assertEqual(first['fields']['id'],65529)
        self.assertEqual(first['fields']['unknown_u32_offset_533'],4294967289)
        self.assertEqual(reconstruct(result,'mark.dat'),bytes(header+wire))
        with self.assertRaisesRegex(ValueError,'version'):
            export.export_records(bytes(553*2),export.FORMATS['mark.dat'])
        with self.assertRaisesRegex(ValueError,'length'):
            export.export_records(bytes(554),export.FORMATS['mark.dat'])

    def test_scene_six_byte_object_prefix(self):
        # Encrypted ID 10000 = (10000 + 9) xor 0xea6c = 0xcd75.
        header=bytearray(131);header[:2]=bytes.fromhex('0400')
        wire=bytearray(131);wire[:2]=bytes.fromhex('75cd')
        wire[2]=3;wire[20:23]=b'cba'
        result=export.export_records(bytes(header+wire),export.FORMATS['scenedata.dat'])
        self.assertEqual(result['records'][0]['fields']['id'],10000)
        self.assertEqual(result['records'][0]['name']['text'],'abc')
        self.assertEqual(len(bytes.fromhex(result['records'][0]['decoded_hex'])),131)

    def test_named_archive_preserves_filename_padding(self):
        record=bytes(range(21))
        native_index=bytes([5])+b'x.wem'+bytes.fromhex('fefdfcfbfaf9f8f7f6f5f4f3f2f1f0')+struct.pack('<II',0,len(record))
        source=record+native_index+bytes.fromhex('0100')
        result=export.export_named(source)
        self.assertEqual(result['entries'][0]['name'],'x.wem')
        self.assertEqual(reconstruct(result,'wem.mmg'),source)
        corrupt=bytearray(source);corrupt[len(record)+21]=1
        with self.assertRaisesRegex(ValueError,'cover payload'):
            export.export_named(corrupt)

    def test_terrain_x_major_grid(self):
        wire=struct.pack('<IIBHHHHH',40,60,1,10000,0,0,2,3)+bytes.fromhex('010203040506')+b'tail'
        result=export.ground_prefix(wire)
        self.assertEqual(result['cell_order'],'x-major')
        self.assertEqual(result['cells_hex'],'010203040506')
        self.assertEqual(result['bytes_read'],len(wire)-4)
        with self.assertRaisesRegex(ValueError,'grid'):
            export.ground_prefix(wire[:-6])

    def test_plaintext_round_trip_and_csv_duplicates(self):
        wire=b'\xef\xbb\xbfName,Value\r\n"a,b",1\r\n"a,b",1\r\n'
        result=export.export_text(wire,'.csv')
        self.assertEqual(result['rows'],[['Name','Value'],['a,b','1'],['a,b','1']])
        self.assertEqual(reconstruct(result,'table.csv'),wire)
        self.assertEqual(result['encoding'],'utf-8-sig')

    def test_client_priority_and_server_only_inventory(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);client=root/'client';server=root/'server'
            client.mkdir();(server/'Data').mkdir(parents=True);(server/'listdata').mkdir()
            (client/'Npc.dat').write_bytes(b'new')
            (server/'Data'/'npc.dat').write_bytes(b'old')
            (server/'Data'/'only.json').write_text('{}')
            (server/'listdata'/'items.csv').write_text('id\n')
            (server/'database.override.txt').write_text('database path')
            (server/'code.cs').write_text('excluded code')
            selected,shadowed=export.inventory(client,server)
            self.assertEqual(selected['npc.dat'],('client',client/'Npc.dat'))
            self.assertEqual(shadowed['npc.dat'],server/'Data'/'npc.dat')
            self.assertEqual(len(selected),3)
            self.assertNotIn('database.override.txt', selected)

    def test_audio_payload_index_and_source_anomaly(self):
        # An independent Ogg page fixture checks framing; its checksum is zero
        # because the exporter preserves source bytes rather than verifying CRCs.
        with tempfile.TemporaryDirectory() as directory:
            source=Path(directory)/'audio.dat';output=Path(directory)/'audio.ogg'
            page=struct.pack('<4sBBQIIIB',b'OggS',0,6,0,1,0,0,1)+b'\x03abc'
            index=bytes([5])+b'x.ogg'+bytes(15)+struct.pack('<II',0,len(page))
            original=page+index+bytes.fromhex('0100')
            source.write_bytes(original)
            result=export.export_audio(source,output,{})
            self.assertEqual(output.read_bytes(),page)
            self.assertEqual(result['entries'][0]['pages'],1)
            self.assertNotIn('source_warnings',result['entries'][0])
            self.assertEqual(output.read_bytes()+bytes.fromhex(result['footer_hex']),original)
            missing_end=bytearray(original);missing_end[5]=2
            source.write_bytes(missing_end)
            result=export.export_audio(source,output,{})
            self.assertIn('source_warnings',result['entries'][0])
            self.assertEqual(source.read_bytes(),missing_end)
            broken_sequence=bytearray(original);broken_sequence[18]=1
            source.write_bytes(broken_sequence)
            with self.assertRaisesRegex(ValueError,'beginning'):
                export.export_audio(source,output,{})

    def test_audio_export_requires_only_original_source(self):
        import extract_audio
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)/'originals';root.mkdir()
            destination=Path(directory)/'outputs'
            page=struct.pack('<4sBBQIIIB',b'OggS',0,6,0,1,0,0,1)+b'\x03abc'
            index=bytes([5])+b'x.ogg'+bytes(15)+struct.pack('<II',0,len(page))
            original=page+index+bytes.fromhex('0100')
            (root/'odd.dat').write_bytes(original)
            extract_audio.extract(root,destination)
            self.assertEqual((destination/'odd'/'x.ogg').read_bytes(),page)
            self.assertFalse((destination/'odd.ogg').exists())
            document=json.loads((destination/'odd_index.json').read_text())
            self.assertEqual(document['track_directory'],'odd')
            self.assertNotIn('payload_file',document)
            manifest=json.loads((destination/'manifest.json').read_text())
            self.assertEqual(Path(manifest['archives'][0]['source']),root/'odd.dat')
            # Exported metadata cannot supply regeneration inputs.
            (destination/'odd_index.json').write_text('invalid cached index')
            extract_audio.extract(root,destination,verify_existing=True)
            self.assertEqual((destination/'odd'/'x.ogg').read_bytes(),page)
            with self.assertRaisesRegex(ValueError,'protect edits'):
                extract_audio.extract(root,destination)

    def test_original_account_database_and_sidecars_are_excluded(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            client = root / 'client'
            server = root / 'server'
            client.mkdir()
            (server / 'Data').mkdir(parents=True)
            for name in ('ServerDatabase.db', 'ServerDatabase.db-wal', 'ServerDatabase.db-shm', 'ServerDatabase.db-journal', 'database.override.txt'):
                (server / 'Data' / name).write_bytes(b'private account data')
                (client / name).write_bytes(b'client private data')
            (server / 'Data' / 'starter_items.json').write_text('[]')
            selected, shadowed = export.inventory(client, server)
            self.assertEqual(set(selected), {'starter_items.json'})
            self.assertEqual(shadowed, {})



if __name__=='__main__':
    unittest.main()

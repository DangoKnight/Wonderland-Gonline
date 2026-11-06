import copy
import struct
import unittest

import extract_igg_translations as igg


class IGGTranslationsTest(unittest.TestCase):
    def test_english_npc_layout_has_twenty_name_bytes(self):
        # Independent English wire fixture: version 12 at 48, ID 10000 at
        # 22 (18 75), type 1 at 21 (ca), body 1170 at 24 (9a 56).
        header=bytearray(148);header[48]=12
        wire=bytearray(148);wire[0]=18
        wire[3:21]=b'reidloS gnitcepsnI'
        wire[21:26]=bytes.fromhex('ca18759a56')
        row=igg.decode_igg('npc',bytes(header+wire))[0]
        self.assertEqual(row['name']['text'],'Inspecting Soldier')
        self.assertEqual(row['fields']['id'],10000)
        canonical=bytes.fromhex(row['decoded_hex'])
        self.assertEqual(len(canonical),138)
        self.assertEqual(int.from_bytes(canonical[14:16],'little'),1170)

    def test_english_item_layout_and_native_data_remain_separate(self):
        # Version 9 at 120; ID 10003 at 22 (df c8); English description is
        # reversed across bytes 153..406, not the Chinese 147..400 range.
        header=bytearray(457);header[120]=9
        wire=bytearray(457);wire[0]=15;wire[6:21]=b"tfaR s'nosniboR"
        wire[22:24]=bytes.fromhex('dfc8');wire[152]=4;wire[403:407]=b'tset'
        row=igg.decode_igg('item',bytes(header+wire))[0]
        self.assertEqual(row['name']['text'],"Robinson's Raft")
        self.assertEqual(row['description']['text'],'test')
        self.assertEqual(row['fields']['id'],10003)
        self.assertEqual(len(bytes.fromhex(row['decoded_hex'])),451)
        header[120]=8
        with self.assertRaisesRegex(ValueError,'unsupported'):
            igg.decode_igg('item',bytes(header+wire))

    def test_same_talk_id_requires_matching_context_and_placeholders(self):
        def row(text):
            return dict(fields={'id':10001},text={'text':text},decoded_hex=bytes(292).hex(),source_hex=bytes(292).hex())
        source={10001:row('Official %s translation')}
        target={10001:row('Old %s translation')}
        original=copy.deepcopy(target)
        rows,report=igg.build_overrides('talk',source,target,({},{}))
        self.assertFalse(rows)
        self.assertEqual(report[0][-1],'no-shared-EVE-dialogue-context')
        contexts=({10001:{('same event',)}},{10001:{('same event',)}})
        rows,_=igg.build_overrides('talk',source,target,contexts)
        self.assertEqual(rows[0][4],'Official %s translation')
        source[10001]['text']['text']='Missing substitution'
        rows,report=igg.build_overrides('talk',source,target,contexts)
        self.assertFalse(rows)
        self.assertEqual(report[0][-1],'placeholder-mismatch')
        self.assertEqual(target,original)

    def test_reused_identity_is_rejected_but_igg_precedes_authored_names(self):
        def row(name,body):
            raw=bytearray(138);struct.pack_into('<H',raw,14,body)
            return dict(fields={'id':11000},name={'text':name},decoded_hex=raw.hex(),source_hex=raw.hex())
        source={11000:row('Different Cat',42)};target={11000:row('Cat',43)}
        rows,report=igg.build_overrides('npc',source,target)
        self.assertFalse(rows);self.assertEqual(report[0][-1],'identity-mismatch')
        target[11000]=row('Cat',42)
        rows,report=igg.build_overrides('npc',source,target,authored_names={('npc',11000)})
        self.assertEqual(rows[0][4],'Different Cat')
        source[11000]['name']['text']='Cat'
        rows,report=igg.build_overrides('npc',source,target,authored_names={('npc',11000)})
        self.assertEqual(rows[0][3:5],['Cat','Cat'])

    def test_event_context_parser_uses_branch_and_operation_not_record_order(self):
        # One player speech operation (code 1 / speech 2), then one text choice.
        event=struct.pack('<H',1)+struct.pack('<HB20sB',8,0,b'event',1)
        event+=bytes([3])+bytes(21)+bytes([1,4])+struct.pack('<B4H3I',1,2,30115,0,0,0,0,0)
        question=struct.pack('<HH20s3sBBHBB',1,9,b'choice',bytes(3),1,2,30116,1,3)
        offsets=[0]*11;offsets[4]=1;offsets[7]=1+len(event)
        raw=b'\0'+event+question+struct.pack('<11I',*offsets)
        contexts=igg.event_talk_contexts([dict(id=12000,decoded_hex=raw.hex())])
        self.assertIn((12000,4,8,3,4,1,0),contexts[30115])
        self.assertIn(('question',12000,9,2,3),contexts[30116])


if __name__ == '__main__':
    unittest.main()

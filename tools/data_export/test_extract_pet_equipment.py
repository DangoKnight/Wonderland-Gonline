from pathlib import Path
import tempfile
import unittest

import extract_pet_equipment as equipment


class PetEquipmentTest(unittest.TestCase):
    def test_normal_reborn_and_missing_gear(self):
        page = '''<tr wire:key="pet-1"><td><a class="pet-name-link">Roca</a></td>
        <td><div>Normal:</div><a href="https://wlohub.com/items/658">Machete</a>
        <div>Reborn:</div><a href="https://wlohub.com/items/2148">Broadsword</a></td></tr>
        <tr wire:key="pet-2"><td><a class="pet-name-link">Robinson</a></td>
        <td><div>Normal:</div>None<div>Reborn:</div>None</td></tr>'''
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'pets.htm').write_text(page, encoding='utf-8')
            pets, sources = equipment.extract(Path(directory))
            self.assertEqual(pets[1], dict(name='Roca', normal=658, reborn=2148))
            self.assertEqual(pets[2], dict(name='Robinson', normal=0, reborn=0))
            self.assertEqual(len(sources), 1)
            Path(directory, 'duplicate.htm').write_text(page, encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'duplicate pet'):
                equipment.extract(Path(directory))

    def test_matching_requires_icon_rank_and_resolves_authored_alias(self):
        raw = bytearray(46)
        raw[equipment.NATIVE_RANK_OFFSET] = 5
        native = [dict(definition=dict(id=15059, name='Thunder Claws', equip_slot=3), icon=55, decoded_hex=raw.hex())]
        ref = dict(name='Thunder Talon', icon='55', rank='5')
        self.assertIsNone(equipment.resolve(ref, native))
        self.assertEqual(equipment.resolve(ref, native, 15059), native[0])
        with self.assertRaisesRegex(ValueError, 'invalid authored'):
            equipment.resolve(dict(ref, icon='56'), native, 15059)


if __name__ == '__main__':
    unittest.main()

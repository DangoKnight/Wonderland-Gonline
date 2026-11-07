from pathlib import Path
import tempfile
import unittest
import zipfile

import bug_compare as bugs


class BugWorkbookTests(unittest.TestCase):
    def test_contact_numbers_are_preserved(self):
        self.assertEqual(bugs.damage_values('316 + 737'), [316,737])
        self.assertEqual(bugs.damage_values('1185'), [1185])
        with self.assertRaises(ValueError):
            bugs.damage_values('likely 1185')

    def test_reads_shared_and_inline_strings_without_modifying_workbook(self):
        namespace = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)/'measurements.xlsx'
            with zipfile.ZipFile(path,'w') as archive:
                archive.writestr('xl/workbook.xml',f'<workbook xmlns="{namespace}" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Measurements" sheetId="1" r:id="rId1"/></sheets></workbook>')
                archive.writestr('xl/_rels/workbook.xml.rels','<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>')
                archive.writestr('xl/sharedStrings.xml',f'<sst xmlns="{namespace}"><si><t>316 + 737</t></si></sst>')
                archive.writestr('xl/worksheets/sheet1.xml',f'<worksheet xmlns="{namespace}"><sheetData><row r="16"><c r="B16" t="inlineStr"><is><t>Water Thunder Attack</t></is></c><c r="C16"><v>1</v></c><c r="E16" t="s"><v>0</v></c></row></sheetData></worksheet>')
            before = path.read_bytes()
            result = bugs.read_xlsx(path)
            self.assertEqual(result,[dict(name='Measurements',cells={'B16':'Water Thunder Attack','C16':'1','E16':'316 + 737'})])
            self.assertEqual(path.read_bytes(),before)


if __name__=='__main__':
    unittest.main()

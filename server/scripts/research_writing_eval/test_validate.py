import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent


class OfflineValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=ROOT)
        self.root = Path(self.temp.name)
        for name in ('manifest.json', 'system-cases.json', 'retrieval-cases.json', 'materials', 'prompts'):
            source = ROOT / name
            if source.is_dir():
                shutil.copytree(source, self.root / name)
            elif source.is_file():
                shutil.copy2(source, self.root / name)

    def tearDown(self):
        self.temp.cleanup()

    def run_validator(self):
        return subprocess.run([sys.executable, '-B', str(ROOT / 'validate.py'), '--root', str(self.root)],
                              text=True, capture_output=True)

    def mutate(self, filename, callback):
        path = self.root / filename
        data = json.loads(path.read_text(encoding='utf-8'))
        callback(data)
        path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

    def test_fixture_passes_and_reports_no_execution(self):
        result = self.run_validator()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('30', result.stdout)
        self.assertIn('BLOCKED', result.stdout)

    def test_scenario_tasks_are_not_label_only_copies(self):
        manifest = json.loads((ROOT / 'manifest.json').read_text(encoding='utf-8'))
        cases = {(case['scenario'], case['stage'], case['condition']): case
                 for case in manifest['eval_cases']}
        for (scenario, stage, condition), intro in cases.items():
            if scenario != 'intro_paper':
                continue
            thesis = cases[('undergraduate_thesis', stage, condition)]
            with self.subTest(stage=stage, condition=condition):
                self.assertNotEqual(intro['task'].strip(), thesis['task'].strip())
                self.assertNotEqual(intro['expected_boundaries'], thesis['expected_boundaries'])

    def test_malformed_manifest_fails(self):
        (self.root / 'manifest.json').write_text('{bad', encoding='utf-8')
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_material_tampering_fails(self):
        with (self.root / 'materials' / 'blueprint.md').open('a', encoding='utf-8') as f:
            f.write('tampered\n')
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_missing_combination_fails(self):
        def mutate(data):
            for case in data['eval_cases']:
                if case['scenario'] == 'intro_paper' and case['stage'] == 'review':
                    case['stage'] = 'write'
        self.mutate('manifest.json', mutate)
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_dev_id_mixed_into_eval_fails(self):
        self.mutate('manifest.json', lambda d: d['eval_cases'][0].update(id=d['dev_cases'][0]['id']))
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_traversal_path_fails(self):
        self.mutate('manifest.json', lambda d: d['materials'][0].update(path='../outside.md'))
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_unknown_reference_fails(self):
        self.mutate('manifest.json', lambda d: d['eval_cases'][0]['materials'].append('missing'))
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_crlf_materials_have_same_digest(self):
        path = self.root / 'materials' / 'blueprint.md'
        path.write_bytes(path.read_bytes().replace(b'\n', b'\r\n'))
        result = self.run_validator()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_duplicate_ids_fail(self):
        self.mutate('manifest.json', lambda d: d['eval_cases'][1].update(id=d['eval_cases'][0]['id']))
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_missing_required_field_fails(self):
        self.mutate('manifest.json', lambda d: d['eval_cases'][0].pop('task'))
        result = self.run_validator()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('missing fields', result.stderr)

    def test_prompt_tampering_fails(self):
        with (self.root / 'prompts' / 'general.txt').open('a', encoding='utf-8') as f:
            f.write('tampered\n')
        result = self.run_validator()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('sha256 mismatch', result.stderr)

    def test_bad_anchor_fails(self):
        self.mutate('manifest.json', lambda d: d['eval_cases'][0]['evidence'][0].update(anchor='BP-NOTFOUND'))
        result = self.run_validator()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('missing or ambiguous anchor', result.stderr)

    def test_no_execution_switch(self):
        result = subprocess.run([sys.executable, '-B', str(ROOT / 'validate.py'), '--execute'],
                                text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)

    def test_external_system_reference_fails(self):
        self.mutate('system-cases.json', lambda d: d['cases'][0]['materials'].append('missing'))
        self.assertNotEqual(self.run_validator().returncode, 0)

    def test_broken_retrieval_evidence_fails(self):
        self.mutate('retrieval-cases.json', lambda d: d['queries'][0].update(relevant=[{'material': 'missing', 'anchor': 'X'}]))
        self.assertNotEqual(self.run_validator().returncode, 0)


if __name__ == '__main__':
    unittest.main()

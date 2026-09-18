"""Verify that the evaluation oracle detects loss, rather than just shorter output."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class RunnerTests(unittest.TestCase):
    def test_unsafe_scores_fail_evidence_and_command_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            scores = Path(directory) / 'unsafe.json'
            scores.write_text(json.dumps({'old-call': {'loss': {
                action: .01 for action in ['extract', 'brief', 'reference', 'drop']
            }}}))
            result = subprocess.run([
                sys.executable, str(ROOT / 'eval/run.py'), '--binary', str(ROOT / 'bin/compactd'),
                '--scores', str(scores),
            ], cwd=ROOT, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 1, result.stderr)
        cases = {case['case']: case for case in json.loads(result.stdout)['cases']}
        self.assertFalse(cases['middle-evidence']['evidence_retained'])
        self.assertFalse(cases['exact-command']['command_exact'])
        self.assertFalse(cases['middle-evidence']['passed'])
        self.assertFalse(cases['exact-command']['passed'])
        self.assertTrue(all(case['recall_exact'] for case in cases.values()))


if __name__ == '__main__':
    unittest.main()

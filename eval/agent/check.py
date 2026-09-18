"""Recheck published agent patches offline. This does not run a model."""

import difflib
import hashlib
import json
from pathlib import Path
import tempfile

from cases import CASES
from run import grade, populate


def main():
    evidence = Path(__file__).resolve().parents[2] / 'docs/results/agent-pilot'
    summary = json.loads((evidence / 'summary.json').read_text())
    expected = {(c['id'], arm, repeat) for c in CASES for arm in ['full', 'compacted']
                for repeat in range(1, summary['repeats_per_arm'] + 1)}
    actual_runs = [(r['case'], r['arm'], r['repeat']) for r in summary['runs']]
    if not expected or set(actual_runs) != expected or len(actual_runs) != len(expected):
        raise RuntimeError('Recorded plan has missing or duplicate runs')
    fixtures = {item['case']: item['fixture_sha256'] for item in summary['preparation']['cases']}
    checked = 0
    with tempfile.TemporaryDirectory(prefix='compact-evidence-') as temporary:
        for case in CASES:
            digest = hashlib.sha256(json.dumps(case, sort_keys=True).encode()).hexdigest()
            if digest != fixtures[case['id']]:
                raise RuntimeError('Fixture changed since recording: ' + case['id'])
            workspace = Path(temporary) / case['id']
            populate(case, workspace)
            if grade(case, workspace)['passed']:
                raise RuntimeError('Buggy fixture unexpectedly passes: ' + case['id'])
            records = [r for r in summary['runs'] if r['case'] == case['id']]
            for record in records:
                source = record['source_after']
                patch = ''.join(difflib.unified_diff(
                    case['source'].splitlines(True), source.splitlines(True),
                    fromfile='a/' + case['module'], tofile='b/' + case['module']))
                saved = (evidence / case['id'] / (record['run_id'] + '.diff')).read_text()
                if patch != saved:
                    raise RuntimeError('Patch differs from recorded source: ' + record['run_id'])
                (workspace / case['module']).write_text(source)
                actual = grade(case, workspace)
                if actual['passed'] != record['contract_tests']['passed']:
                    raise RuntimeError('Contract result changed: ' + record['run_id'] + '\n' + actual['output'])
                checked += 1
    print(f'Rechecked {checked} recorded patches and {len(CASES)} failing baselines without inference calls.')


if __name__ == '__main__':
    main()

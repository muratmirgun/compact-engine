"""Evaluate synthetic transcripts. Replay checks mechanics; live checks a scorer."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile


def run(binary, request_path, archive, scores, model):
    command = [str(binary), 'compact', '-input', str(request_path), '-archive', archive]
    command += ['-model', model]
    if scores:
        command += ['-scores', str(scores)]
    completed = subprocess.run(command, capture_output=True, text=True, check=True, timeout=45)
    return json.loads(completed.stdout)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--live', action='store_true')
    mode.add_argument('--replay', action='store_true', help='Use per-case synthetic scores without network access.')
    mode.add_argument('--scores', type=Path, help='Offline replay; does not assess model quality.')
    parser.add_argument('--model', default='jev-latest')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    fixtures = Path(__file__).resolve().parent / 'fixtures'
    cases = [
        ('original', fixtures / 'original.json', 'compacted'),
        ('middle-evidence', fixtures / 'middle-evidence.json', 'compacted'),
        ('partial-budget', fixtures / 'partial-budget.json', 'partial'),
        ('exact-command', fixtures / 'exact-command.json', 'budget_unmet'),
    ]
    replay = json.loads((fixtures / 'replay-scores.json').read_text()) if args.replay else {}
    reports = []
    with tempfile.TemporaryDirectory(prefix='compact-eval-') as archive:
        for name, path, expected in cases:
            request = json.loads(path.read_text())
            scores = args.scores
            if args.replay:
                scores = Path(archive) / 'scores.json'
                scores.write_text(json.dumps(replay[name]))
            result = run(binary, path, archive, scores, args.model)
            by_id = {m['id']: m for m in result['messages']}
            protected = all(
                by_id.get(m['id']) == m for m in request['messages']
                if m['id'] in {'rules', 'goal', 'test-call', 'test-result'}
            )
            combined = '\n'.join(m['text'] for m in result['messages'])
            evidence = name != 'middle-evidence' or 'cache:<tenant>:<id>' in combined
            exact = name != 'exact-command' or by_id.get('old-result') == request['messages'][3]
            recalled = subprocess.run(
                [str(binary), 'recall', '-archive', archive, '-snapshot', result['snapshot_id'], '-message', 'old-result'],
                capture_output=True, text=True, check=True, timeout=15,
            )
            recall_exact = json.loads(recalled.stdout) == request['messages'][3]
            passed = protected and evidence and exact and recall_exact and result['status'] == expected
            reports.append({
                'case': name, 'passed': passed, 'expected_status': expected,
                'status': result['status'], 'stats': result['stats'],
                'scores': result.get('scores', {}), 'protected_exact': protected,
                'evidence_retained': evidence, 'command_exact': exact, 'recall_exact': recall_exact,
                'input_sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
            })
    report = {'schema_version': 1, 'mode': 'live' if args.live else 'replay',
              'requested_model': args.model if args.live else None, 'cases': reports}
    rendered = json.dumps(report, indent=2) + '\n'
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered)
    print(rendered, end='')
    # Failure is useful evidence. Never relabel unchanged history as compaction.
    return 0 if all(r['passed'] for r in reports) else 1


if __name__ == '__main__':
    raise SystemExit(main())

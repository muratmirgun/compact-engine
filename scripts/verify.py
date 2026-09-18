"""Run local release checks and preserve their evidence. No inference API calls."""

from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys

from check_repo import ROOT, repository_files


def execute(command, log):
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=300)
    (ROOT / 'reports' / log).write_text(result.stdout + result.stderr)
    print(f'{"PASS" if result.returncode == 0 else "FAIL"}: {" ".join(command)}', flush=True)
    if result.returncode:
        raise RuntimeError(f'Check failed; see reports/{log}')
    return result.stdout


def main():
    (ROOT / 'reports').mkdir(exist_ok=True)
    formatting = execute(['gofmt', '-l', '.'], 'format.txt')
    if formatting.strip():
        raise RuntimeError('Unformatted Go files; see reports/format.txt')
    execute([sys.executable, 'scripts/check_repo.py'], 'repository.txt')
    execute(['go', 'vet', './...'], 'vet.txt')
    events = execute(['go', 'test', '-count=1', '-json', '-race', '-coverprofile=reports/coverage.out', '-covermode=atomic', './...'], 'tests.jsonl')
    coverage = execute(['go', 'tool', 'cover', '-func=reports/coverage.out'], 'coverage.txt')
    execute(['golangci-lint', 'run', './...'], 'lint.txt')
    execute(['go', 'build', '-trimpath', '-o', 'bin/compactd', './cmd/compactd'], 'build.txt')
    execute([sys.executable, '-m', 'unittest', 'discover', '-s', 'eval', '-p', 'test_*.py'], 'evaluation-oracle.txt')
    execute([sys.executable, 'eval/agent/check.py'], 'agent-evidence.txt')
    execute([sys.executable, 'eval/run.py', '--binary', 'bin/compactd', '--replay', '--output', 'reports/replay.json'], 'replay.txt')
    tests = [json.loads(line) for line in events.splitlines() if line.strip()]
    passed = [event for event in tests if event.get('Action') == 'pass' and event.get('Test')]
    packages = [event['Package'] for event in tests if event.get('Action') == 'pass' and not event.get('Test')]
    versions = {
        'go': subprocess.check_output(['go', 'version'], cwd=ROOT, text=True).strip(),
        'linter': subprocess.check_output(['golangci-lint', 'version'], cwd=ROOT, text=True).strip(),
        'python': platform.python_version(),
        'os': platform.system(), 'architecture': platform.machine(),
    }
    hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
              for p in repository_files() if p.suffix in {'.go', '.py'} or p.name in {'go.mod', 'go.sum'}}
    summary = {
        'schema_version': 1, 'completed_at': datetime.now(timezone.utc).isoformat(),
        'versions': versions, 'packages_passed': packages,
        'top_level_tests_passed': sum('/' not in e['Test'] for e in passed),
        'test_events_passed_including_subtests': len(passed),
        'coverage_total': coverage.strip().splitlines()[-1].split()[-1],
        'replay_cases_passed': sum(c['passed'] for c in json.loads((ROOT / 'reports/replay.json').read_text())['cases']),
        'source_sha256': hashes,
    }
    (ROOT / 'reports/summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    print('Verification complete. Evidence: reports/summary.json')
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (OSError, subprocess.SubprocessError, RuntimeError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)

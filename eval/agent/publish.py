"""Publish sanitized pilot evidence and the data consumed by the visual replay."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import statistics

from cases import CASES


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepared', type=Path, required=True)
    parser.add_argument('--runs', type=Path, required=True)
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[2]
    destination = project / 'docs/results/agent-pilot'
    destination.mkdir(parents=True, exist_ok=True)
    summary = json.loads((args.runs / 'summary.json').read_text())
    prep = json.loads((args.prepared / 'preparation.json').read_text())
    demo = {'model': summary['model'], 'case_count': len(CASES), 'run_count': len(summary['runs']), 'cases': []}
    titles = {'tenant-cache': 'Tenant cache', 'retry-policy': 'Retry policy', 'cursor-pagination': 'Cursor pagination'}
    summary = json.loads(json.dumps(summary).replace(str(args.runs.resolve()), '<runs>'))
    published = dict(summary)
    for record in published['runs']:
        record['source_after'] = (args.runs / record['run_id'] / 'workspace' / next(
            c['module'] for c in CASES if c['id'] == record['case']
        )).read_text()
        # These fields contain commands and final text, never model reasoning.
        encoded = json.dumps(record).replace(str(args.runs.resolve()), '<runs>')
        if re.search(r'/Users/[A-Za-z0-9_.-]+/', encoded):
            raise RuntimeError('Personal path remains in public run data: ' + record['run_id'])
    for case in CASES:
        case_id = case['id']
        folder = destination / case_id
        folder.mkdir(exist_ok=True)
        request = json.loads((args.prepared / case_id / 'request.json').read_text())
        result = json.loads((args.prepared / case_id / 'result.json').read_text())
        initial = json.loads((args.prepared / case_id / 'initial-grade.json').read_text())
        initial['output'] = initial['output'].replace(str((args.prepared / case_id / 'initial').resolve()), '<initial-workspace>')
        for name, value in [('request.json', request), ('compaction.json', result), ('initial-tests.json', initial)]:
            (folder / name).write_text(json.dumps(value, indent=2) + '\n')
        records = [r for r in summary['runs'] if r['case'] == case_id]
        for record in records:
            (folder / (record['run_id'] + '.json')).write_text(json.dumps(record, indent=2) + '\n')
            patch = (args.runs / record['run_id'] / 'patch.diff').read_text()
            (folder / (record['run_id'] + '.diff')).write_text(patch)
        original = next(m for m in request['messages'] if m['id'] == 'investigation-result')['text']
        reduced = next(m for m in result['messages'] if m['id'] == 'investigation-result')['text']
        if not all(finding in reduced for finding in case['findings']):
            raise RuntimeError('A finding is absent; do not render it as preserved: ' + case_id)
        archive_file = args.prepared / 'archive' / (result['snapshot_id'] + '.json')
        saved = archive_file.read_bytes()
        recall_exact = hashlib.sha256(saved).hexdigest() == result['snapshot_id'] and json.loads(saved) == request
        full = [r for r in records if r['arm'] == 'full']
        compact = [r for r in records if r['arm'] == 'compacted']
        selected_run = sorted(compact, key=lambda r: r['repeat'])[0]
        entry = {
            'id': case_id, 'title': titles[case_id], 'goal': case['goal'], 'module': case['module'],
            'before': result['stats']['input_tokens'], 'after': result['stats']['output_tokens'],
            'compaction_ms': result['stats']['total_ms'], 'snapshot': result['snapshot_id'],
            'original_lines': len(original.splitlines()), 'original_head': original.splitlines()[:2],
            'retained_lines': len(re.findall(r'^L\d+:', reduced, re.MULTILINE)),
            'findings': case['findings'], 'recall_exact': recall_exact,
            'scores': result['scores']['investigation-call']['loss'],
            'action': next(d['action'] for d in result['decisions'] if d['group_id'] == 'investigation-call'),
            'patch': (args.runs / selected_run['run_id'] / 'patch.diff').read_text(),
            'test_count': len(re.findall(r'def test_', case['grader'])),
            'full_passed': sum(r['passed'] for r in full), 'full_total': len(full),
            'compact_passed': sum(r['passed'] for r in compact), 'compact_total': len(compact),
        }
        demo['cases'].append(entry)
    metrics = {}
    for arm in ['full', 'compacted']:
        runs = [r for r in summary['runs'] if r['arm'] == arm]
        metrics[arm] = {
            'passed': sum(r['passed'] for r in runs), 'runs': len(runs),
            'median_seconds': statistics.median(r['elapsed_seconds'] for r in runs),
            'total_seconds': sum(r['elapsed_seconds'] for r in runs),
            'input_tokens': sum(r['usage'].get('input_tokens', 0) for r in runs),
            'cached_input_tokens': sum(r['usage'].get('cached_input_tokens', 0) for r in runs),
            'output_tokens': sum(r['usage'].get('output_tokens', 0) for r in runs),
            'recall_related_commands': sum(r['recall_commands'] for r in runs),
        }
    published['metrics'] = metrics
    published['preparation'] = prep
    published['billed_cost_measured'] = False
    published['limitations'] = [
        'Three authored Python tasks with two repetitions per arm; no statistical generalization.',
        'Fresh Codex prompts receive serialized checkpoint messages; this does not replace native Codex session compaction.',
        'Jev compaction was prepared once per task and reused across continuations.',
        'Agent latency excludes the separately reported compaction pass.',
        'CLI token usage includes system instructions, tool calls, caching, and multiple inference steps.',
        'The scorer resolved model version was not recorded; the requested alias was jev-latest.',
        'No hidden-test file was placed inside agent workspaces; filesystem access isolation was not an adversarial containment test.',
    ]
    (destination / 'summary.json').write_text(json.dumps(published, indent=2) + '\n')
    (project / 'demo/data.js').write_text('window.COMPACT_DEMO = ' + json.dumps(demo, indent=2) + ';\n')
    print(json.dumps(metrics, indent=2))


if __name__ == '__main__':
    main()

# Real-agent pilot

On 2026-09-18, Codex completed 12 continuations across three authored Python tasks.
Full history passed **6/6** runs. Compacted history passed **6/6** runs.
Each run passed five held-out behavior tests and the file-change constraint.
This small pilot demonstrates working continuations. It does not establish a general success rate or cost advantage.

## Task results

| Task | Checkpoint tokens | Reduction | Live compaction | Full history | Compacted history |
|---|---:|---:|---:|---:|---:|
| Tenant cache | 2,617 → 634 | 75.8% | 1.081 s | 2/2 | 2/2 |
| Retry policy | 2,648 → 657 | 75.2% | 1.251 s | 2/2 | 2/2 |
| Cursor pagination | 2,641 → 637 | 75.9% | 1.010 s | 2/2 | 2/2 |

These token counts use the engine's canonical estimate. They exclude the surrounding Codex prompt and subsequent agent work.
Jev selected literal excerpts in all three cases. Original archive bytes matched their hashes and requests.
The scorer ran once per task. Both repetitions used the same selected history.

## Total agent work

| Metric | Full history, six runs | Compacted history, six runs |
|---|---:|---:|
| Passed runs | 6 | 6 |
| Median continuation time | 45.19 s | 43.64 s |
| Total continuation time | 260.64 s | 261.50 s |
| Reported input tokens | 427,191 | 433,654 |
| Cached input tokens, included above | 325,504 | 328,320 |
| Reported output tokens | 6,014 | 6,327 |
| Commands that referenced recall | 0 | 0 |

**The smaller checkpoints did not reduce total agent tokens in this pilot.**
Compacted runs used 1.5% more reported input tokens and 5.2% more output tokens.
CLI usage includes system instructions, tools, cache effects, and multiple inference steps.
The timing differences are too small for a speed claim with this sample.
Continuation times exclude compaction. Add its separately reported latency when assessing a new compaction event.
We did not measure billed cost.

## Method

- Model: `gpt-6-astra`, reasoning effort `high`.
- Codex CLI: `0.155.0-alpha.2.6`, authenticated through an existing ChatGPT login.
- Environment: macOS arm64, Python 3.14.7, isolated directories, `workspace-write` sandbox.
- Scorer: live `jev-latest`; the provider's resolved model version was not recorded.
- Plan: three tasks, two arms, two repetitions, randomized order with seed `731`.
- Limit: 180 seconds per continuation. No failed run was silently replaced.

The harness supplied authored checkpoint messages as JSON inside a fresh Codex prompt.
It did not replace native Codex session compaction or resume opaque model state.
Both arms received the same buggy source, visible test, task instructions, and archive recall helper.
The compacted arm received the engine's returned messages.

Each task started with failing contract tests.
Codex then edited actual source files and ran local tests.
After each continuation, the evaluator supplied five additional tests to Python through standard input.
Those test files never appeared inside the agent workspace.
The evaluator also required changes to exactly the target module.
Archive changes, generated file changes, and test edits would fail that constraint.

The tasks cover tenant isolation, return values, retry limits, backoff, cursor boundaries, duplicate items, and cycle detection.
The findings appeared in the middle of a longer investigation log.
This is an authored pilot, not a held-out production benchmark.
The directory restrictions do not constitute an adversarial filesystem containment test.
No run invoked recall, so the experiment does not measure recovery behavior during difficult agent work.

## Inspect the evidence

- [Aggregate and individual run records](agent-pilot/summary.json).
- [Tenant cache input](agent-pilot/tenant-cache/request.json), [selected history](agent-pilot/tenant-cache/compaction.json), and [actual patch](agent-pilot/tenant-cache/tenant-cache-compacted-1.diff).
- [Retry input](agent-pilot/retry-policy/request.json), [selected history](agent-pilot/retry-policy/compaction.json), and [actual patch](agent-pilot/retry-policy/retry-policy-compacted-1.diff).
- [Pagination input](agent-pilot/cursor-pagination/request.json), [selected history](agent-pilot/cursor-pagination/compaction.json), and [actual patch](agent-pilot/cursor-pagination/cursor-pagination-compacted-1.diff).
- [Runnable fixtures and held-out checks](../../eval/agent/cases.py).
- [Interactive replay](../../demo/index.html) and [demo instructions](../../demo/README.md).

Each task directory includes all four run records, all four patches, and its failing initial tests.
Public records contain commands, final answers, timings, and test output.
They also include the resulting source, so offline checks can repeat the contract tests on every published patch.
They omit raw reasoning streams, credentials, and session identifiers.
The demo consistently displays the first compacted repetition for each task.

## Reproduce

Verify the published code changes without any model calls:

```sh
python3 eval/agent/check.py
```

This checks all twelve recorded patches and three failing baselines. `make verify` also runs this check.

Install Codex CLI and `rtk`. Authenticate Codex before running the experiment.
Use an account that supports the requested model.
Set `TYPESAFE_API_KEY` through your environment for live preparation.
The preparation uses provider inference. Continuations consume Codex account usage.

Run these commands from the repository root:

```sh
make build
python3 eval/agent/run.py prepare --binary bin/compactd --output reports/agent-prepared
python3 eval/agent/run.py run \
  --prepared reports/agent-prepared --output reports/agent-runs \
  --model gpt-6-astra --effort high --repeats 2 --seed 731 --timeout 180
```

The runner rejects a nonempty run directory. Use a new directory for another experiment.
Provider aliases, cache conditions, and model availability can change the result.
The [larger evaluation protocol](../evaluation.md#paired-agent-evaluation-protocol) specifies broader tasks and independent evaluation before threshold tuning.

To replace the recorded evidence and demo data after reviewing a new run:

```sh
python3 eval/agent/publish.py --prepared reports/agent-prepared --runs reports/agent-runs
```

This command writes repository files. Review all results, including failures, before publication.

# Recorded results

This report separates implementation checks from semantic and agent quality.
The [real-agent pilot](agent-pilot.md) includes twelve continuations across three authored tasks.
Both arms passed all six runs. Smaller checkpoints did not reduce total agent token usage.

## Local verification

The publication check runs race tests, coverage, vet, lint, a build, repository checks, and four offline cases.
The local check completed on macOS arm64 with Go 1.26.4 and golangci-lint 2.12.2.

| Check | Observed result |
|---|---|
| Race tests | Six packages passed; 30 top-level checks, including example and fuzz seed targets. |
| Statement coverage | 84.4% overall; 92.8% in `compact`, 90.1% in `server`. |
| Vet and lint | Passed; zero lint findings. |
| Offline evaluation | Four of four declared cases passed. |
| Evaluation oracle | Deliberately unsafe scores correctly failed evidence and exact-command checks. |
| Recorded agent patches | Twelve patches repeated their contract results; all three initial fixtures still fail. |
| Excerpt fuzzing | 262,933 executions passed. |
| Transcript fuzzing | 24,747 executions passed. |
| Build and local links | Passed. |

Fuzz execution counts reflect this run and its available corpus; they are not coverage percentages.
Coverage measures exercised statements, not semantic accuracy.
The archive and CLI packages have lower coverage; filesystem fault injection and additional CLI error paths remain useful work.

[Verification manifest](raw/summary.json), [test events](raw/tests.jsonl),
[statement coverage](raw/coverage.txt), [offline cases](raw/replay.json),
[fuzz output](raw/fuzz.txt), and [oracle check](raw/evaluation-oracle.txt) preserve the evidence.
Use `make verify` to regenerate them under `reports/`.

The core tests cover protected state, dependency groups, whole tool exchanges, score validation, input immutability, and cancellation.
HTTP tests cover authentication, schema limits, overload, error mapping, snapshots, and recovery.
Archive tests cover exact recovery, corruption detection, permissions, and concurrent writes.
Fuzz tests check UTF-8 excerpts and generated transcript invariants.

GitHub CI is configured for Linux and macOS.
Inspect the [GitHub checks](https://github.com/muratmirgun/compact-engine/actions/workflows/check.yml) for hosted run status.
The figures in this report describe the recorded local runs.

## Local replay benchmark

Six samples used Apple M1 Pro, Go 1.26.4, and a synthetic 1,000-line tool output.
The median was **14.091 ms**, with a range of **13.692–14.406 ms**.
The measurement includes tokenization, archive writes, selection, and HTTP handler encoding.
It excludes live scoring, actual socket transfer, and agent continuation.
No comparative speed claim is made.

[Raw benchmark output](raw/benchmark.txt) includes allocation counts and the command.

## Live synthetic observations

Recorded on 2026-09-18 with the requested model `jev-latest`.
The resolved model version was not recorded for these four calls.
Each case ran once. These are development fixtures, not a held-out dataset.

| Case | Target | Input → output | Result | Total time |
|---|---:|---:|---|---:|
| Old build log | 500 | 2,580 → 2,580 | Reduction target missed. | 1,071 ms |
| Important middle finding | 500 | 2,619 → 343 | Brief record; target met. | 1,040 ms |
| Partial-budget request | 100 | 2,580 → 2,580 | No permitted reduction. | 1,107 ms |
| Exact rollback command | 180 | 1,037 → 1,037 | Full command preserved. | 813 ms |

Two cases met their declared expectations; two did not.
This count is not an agent success rate.
The successful reduction retained the tenant cache key finding and the unresolved test failure.
Protected messages remained exact in all four cases.
Original tool outputs were separately recovered from all four archives without changes.

[Engine-side observations](raw/live-final.json) include scores, fixture hashes, and individual timings.
The file contains observed engine data, not raw provider response bodies.
The fixtures now live in `eval/fixtures/` with their original input bytes preserved.
The current verification suite does not make new live API calls.

## Limits exposed by the experiment

| Case | Extract loss | Brief loss | Reference loss | Drop loss |
|---|---:|---:|---:|---:|
| Old log | 0.30 | 0.26 | 0.45 | 0.44 |
| Important finding | 0.18 | 0.19 | 0.59 | 0.82 |
| Partial budget | 0.30 | 0.24 | 0.44 | 0.45 |
| Exact command | 0.23 | 0.43 | 0.81 | 0.92 |

The old log's brief record scored 0.26, while a truncated command scored 0.23.
Simply raising the 0.20 threshold would not reliably distinguish these cases.
Budget checks can provide additional protection, but they do not establish semantic correctness.
Scores are estimates, not calibrated error probabilities.

During development, three long-prompt cases failed with partial previews.
Three more failed after increasing the preview limit.
The final four calls used a shorter question about missing task-relevant facts.
Ten live scoring calls were made during that selection experiment.
Earlier credential checks are excluded from that count.
No statistical attribution or billed cost measurement was completed.

## Reproduction and interpretation

```sh
make verify
make fuzz
make bench
python3 eval/run.py --binary ./bin/compactd --live --output reports/live.json
```

Only the last command uses the provider and requires `TYPESAFE_API_KEY`.
Keep failed live cases in reports. Do not replace them with offline scores and label the result live.
Token counts use canonical message JSON and explicit framing, rather than exact provider billing.
The [evaluation guide](../evaluation.md) describes the completed pilot and the larger evaluation still needed.

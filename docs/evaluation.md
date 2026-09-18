# Evaluation

Compaction is useful when an agent completes its task with less total cost or time.
A smaller transcript alone does not prove that result.

## Evidence levels

| Level | Current coverage | What it does not prove |
|---|---|---|
| Unit and integration tests | Validation, protection, archive integrity, failure behavior, HTTP and CLI contracts. | Semantic quality. |
| Fuzz tests | UTF-8 excerpts and transcript invariants over generated inputs. | Every possible input is safe. |
| Offline replay | Four repeatable transcript cases with declared scores. | Model judgment or provider latency. |
| Live synthetic evaluation | Four cases using Jev, with successes and failures retained. | Real coding task completion. |
| Paired agent evaluation | Three authored tasks; 6/6 full and 6/6 compacted continuations passed. | General task success, native session compaction, or cost savings. |

## Run the synthetic suite

Requirements: Go 1.26+, Python 3.9+, and no Python packages.

```sh
make build
make eval
```

`make eval` runs without credentials or provider calls.
It uses declared fixture scores from `eval/fixtures/replay-scores.json`.
These scores test selection behavior, not model quality.
The `middle-evidence` scores reproduce one recorded live decision.
The other replay scores are deliberately synthetic.

To evaluate the provider, set `TYPESAFE_API_KEY` in your environment and run:

```sh
python3 eval/run.py --binary ./bin/compactd --live --output reports/live.json
```

This makes four paid compaction passes and performs local archive recovery.
Large future fixtures can require multiple provider batches per pass.
There are no automatic retries.
Use `--model` to select a provider model ID.
An alias such as `jev-latest` can change between runs.

The runner exits with code 1 when a case misses any expectation.
Keep failed cases in the report.
Fixture hashes, scores, token counts, and latency accompany the results.
Temporary archives are removed after the run.

| Case | Expected behavior |
|---|---|
| `original` | Reduce an old build log to the target. |
| `middle-evidence` | Reduce the log while retaining the tenant cache key finding. |
| `partial-budget` | Apply permitted reductions, while reporting the remaining overflow. |
| `exact-command` | Preserve the complete command when truncated alternatives lose arguments. |

The runner also checks protected messages and exact archive recovery.
Literal evidence checks are narrow oracles; they do not replace task completion tests.
The live baseline misses two expectations. That limitation is intentional and visible.

## Repeatable repository verification

```sh
make verify
make fuzz
make bench
```

`make verify` writes coverage, test events, tool versions, and file hashes under `reports/`.
It also runs documentation checks, lint, a CLI build, and offline evaluation.
It makes no inference API calls. Go may download missing dependencies.
Reports are ignored by Git until a maintainer selects a reviewed result to publish.

The benchmark includes counting, archive writes, selection, and HTTP handler encoding.
It uses replay scores and an in-memory HTTP recorder.
It excludes provider latency, real socket transfer, and agent continuation.
Use at least six samples, and do not claim improvements from small median differences.

## Paired agent evaluation protocol

Choose checkpoints where compaction is needed, but full history still fits the control model's context window.
Store the repository revision, working changes, transcript, goal, and tool state for each checkpoint.
Use isolated workspaces for each continuation.

Compare two arms:

1. Continue with the complete history.
2. Continue with Compact Engine's selected history and a working recall tool.

Hold the model version, tool definitions, continuation budget, and environment constant.
Randomize arm order to reduce cache and service timing bias.
Record whether provider caching applies.
If a control transcript cannot fit, report it separately or use an explicit native compaction baseline.
Do not silently truncate the control.

Start with ten tasks and three repeats per arm: sixty continuations.
Include tasks with changing requirements, stale outputs, important middle lines, unresolved failures, dependencies, and exact commands.
Freeze a separate evaluation set before tuning prompts or thresholds.
Use public fixtures or obtain permission before sending private transcripts to a provider.

Define success before each run:

- Required tests pass, including held-out checks where possible.
- Existing behavior does not regress.
- Forbidden files and user constraints remain respected.
- The agent produces the required artifact or change.

Measure each arm:

- Task pass rate and constraint violations.
- End-to-end duration to a correct result.
- Total token usage and actual billed cost across all models.
- Compaction latency and provider failures.
- Recall count and repeated tool work.
- Compaction failures, including requests that retain the original history.

Inspect cases where full history passes and compacted history fails.
Trace the failure to missing evidence before attributing it to compaction.
Both arms failing is not direct evidence of compaction damage.
Report paired differences with uncertainty intervals and all task counts.
Do not define an acceptance margin after seeing the results.

The [recorded pilot](results/agent-pilot.md) completed three tasks with two repetitions per arm.
It is smaller than the proposed ten-task evaluation and uses authored checkpoints.
Its passing runs do not establish statistical equivalence or a calibrated selection threshold.

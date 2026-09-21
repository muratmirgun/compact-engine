# Scoring and independent results

The default grouping contract remains unchanged. Set
`Request.ReduceResultsIndividually` to reduce eligible results separately.
This mode preserves every call record and never proposes `drop`.
It preserves side-effect results, errors, recent messages, and explicit dependencies.
A write call no longer prevents reduction of an unrelated read result in the same assistant message.

Jev defaults to two concurrent requests per scoring pass. `MaxConcurrent` accepts 1–8.
Every worker stops before the pass returns. Cancellation or one failed batch invalidates the pass.
Results merge in batch order. The client performs no automatic paid retries.

`MaxRequestTokens` defaults to 8000 and accepts 256–30000.
The existing 24000-byte default remains a separate limit.
Token sizing uses o200k_base over the serialized request, including questions.
This estimate does not claim to reproduce Jev billing or its tokenizer.
Scoring context shrinks before candidate previews. Transcript text remains unchanged.
Exact replacement variants never shrink to fit a request.

`ScoreWithStats` returns per-invocation diagnostics without shared mutable state.
`Stats.Scoring.Requests` counts planned batches, including batches canceled before dispatch.
Protected, kept, reduced, dropped, and rejected group counts describe the proposal.
Always check `Result.Applied` before treating a proposed reduction as an applied change.
Rejected groups have no permitted reduction under the selected scoring policy.

## Local measurements

A six-batch loopback fixture adds 10 ms latency per HTTP response.
Three runs of ten iterations measured about 68–70 ms with one request at a time,
and 37–39 ms with two requests. These are synthetic measurements, not Jev service timings.
Run `go test ./jev -run '^$' -bench BenchmarkScoringBatches -benchtime=10x -count=3`.

The design draws inspiration from tamaratran/fast-jev-compaction:
https://github.com/tamaratran/fast-jev-compaction/tree/e3f262a7f4d42bd8dd32ced30d26176f7cb545b0
This implementation retains Compact Engine archives, protocol validation, and side-effect protection.

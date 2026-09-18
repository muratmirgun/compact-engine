# Contributing

Compact Engine is experimental. Contributions should preserve explicit failure behavior and reproducible evidence.

## Local setup

Install Go 1.26+, Python 3.9+, and golangci-lint 2.12.2.
No provider account is required for development or CI.

```sh
make verify
make fuzz
```

Use `make fmt` to format Go files.
Verification writes ignored artifacts under `reports/`.

## Changes

- Keep core selection independent of providers and HTTP.
- Preserve call/result relationships and declared protection rules.
- Return errors instead of silently discarding unsupported content.
- Add regression tests for behavior changes.
- Update the API guide when request or result semantics change.
- Include failed evaluation cases when changing prompts or thresholds.
- Avoid new dependencies without a concrete need.

Keep pull requests focused. Describe the problem, resulting behavior, and checks performed.
Do not include credentials, private transcripts, archive snapshots, binaries, or machine-specific paths.
Follow the [security policy](SECURITY.md) for vulnerability reports.

## Performance and quality claims

Use at least six benchmark samples under comparable conditions.
State whether measurements include provider calls and archive writes.
Replay scores test mechanics; they do not establish semantic quality.
Use a separate evaluation set when tuning selection behavior.
See the [evaluation protocol](docs/evaluation.md).

## License

Contributions are accepted under [Apache-2.0](LICENSE).
Preserve applicable third-party notices when adding dependencies or borrowed code.

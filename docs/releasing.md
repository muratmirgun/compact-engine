# Release checklist

The source repository is [muratmirgun/compact-engine](https://github.com/muratmirgun/compact-engine).
The project is experimental and has no release tag yet.

## Prepare a source update

1. Keep `github.com/muratmirgun/compact-engine` as the module path.
2. Keep internal imports and documentation examples consistent with that path.
3. Run `go mod tidy` and `make verify`.
4. Run `make fuzz` and `make bench`.
5. Refresh the published evidence and source hashes.
6. Review the changes and create the requested commit.

## Before public release

- Confirm the Apache-2.0 license and third-party notices.
- Review all published files for credentials and private transcripts.
- Enable private vulnerability reporting or document a working private contact.
- Require the Linux and macOS CI jobs to pass.
- Keep failed live evaluation cases visible in the report.
- Label the first release as experimental.
- Do not claim improved task success from the small authored pilot.

Check the [GitHub workflow](https://github.com/muratmirgun/compact-engine/actions/workflows/check.yml) on the exact commit before a release.

## Evidence publication

Run `make verify` to create fresh local artifacts under `reports/`.
Review the files before copying selected evidence into `docs/results/raw/`.
Keep the tool versions, exact commands, fixture hashes, failed cases, and relevant source hashes.
Do not publish private prompts, archive directories, keys, or signed provider URLs.

Use a release tag only after the final module path and verification results match the committed source.

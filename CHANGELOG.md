# Changelog

## v0.3.0

- Add bounded concurrent Jev requests (default two, configurable from one to eight).
- Fit requests to both byte limits and an explicit o200k_base token estimate.
- Build chronological scoring context with tool identities and result metadata.
- Add opt-in independent result reduction without removing call records.
- Preserve side effects, errors, explicit dependencies, and recent messages.
- Return per-pass request counts and proposed reduction diagnostics.
- Test overlapping requests, cancellation, Unicode fitting, and mixed call protection.


## Unreleased

- Added twelve real Codex continuations with paired histories, held-out tests, and published patches.
- Added an interactive replay, GIF, and MP4 based on recorded compaction decisions.

- Added a shared Go core with HTTP, CLI, and library interfaces.
- Added archive storage with integrity checks and exact message recovery.
- Added dependency grouping and explicit message protection.
- Added Jev scoring of concrete replacement alternatives.
- Added optional partial reductions and diagnostic score output.
- Added offline evaluation, race tests, fuzzing, and benchmark evidence.
- Added Apache-2.0 licensing, contribution guidance, and publication documentation.

The API is experimental. No stable release or compatibility promise has been made.

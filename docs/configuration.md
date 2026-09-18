# Configuration

## Commands

| Command | Input | Output |
|---|---|---|
| `compactd serve` | HTTP requests. | HTTP responses; process logs on stderr. |
| `compactd compact` | JSON file or stdin. | One JSON result on stdout. |
| `compactd recall` | Snapshot and optional message ID. | Original request or message on stdout. |

Use `compactd <command> -h` to list its flags.
`compact -input -` reads stdin.
Invalid input or operational errors produce a nonzero exit code.
Valid budget and scorer failure results still use exit code zero.

## Environment

| Variable | Purpose |
|---|---|
| `TYPESAFE_API_KEY` | Provider credential for live scoring; not required in replay mode. |
| `COMPACT_API_TOKEN` | Service Bearer token; optional on loopback, required by the CLI for public binding. |

Do not store credentials in request JSON or score files.
The service does not load `.env` files automatically.
Use your shell environment or a secret manager.

## Flags

| Flag | Default | Scope |
|---|---|---|
| `-archive` | `.compact-data` | Snapshot directory. |
| `-encoding` | `o200k_base` | Also accepts `cl100k_base`. |
| `-scores` | Empty | Explicit offline score JSON; bypasses Jev. |
| `-model` | `jev-latest` | Requested Jev model. |
| `-jev-endpoint` | `https://api.typesafe.ai/v1/systemone` | Provider endpoint; HTTP permitted only on loopback. |
| `-listen` | `127.0.0.1:8787` | HTTP service bind address. |
| `-concurrency` | `4` | Maximum active HTTP data requests. |
| `-input` | `-` | Compact command input file or stdin. |
| `-snapshot` | Required for recall | Snapshot identifier. |
| `-message` | Empty | Recall one message; otherwise recall the whole request. |

## Limits

| Boundary | Default |
|---|---:|
| HTTP request body | 8 MiB |
| Messages per request | 10,000 |
| Goal length | 16,000 bytes |
| Original preview per group | 16,000 bytes |
| Sampled global context | 4,000 bytes |
| Jev request body | 24,000 bytes |
| Jev batches per pass | 32 |
| Jev request timeout | 10 seconds |
| HTTP processing timeout | 30 seconds |

Jev request size limits measure bytes, not model tokens.
Oversized candidates and malformed answers produce a scorer failure result.
Provider requests do not follow redirects or retry automatically.
Custom Go callers can configure transport and server limits through config structs.

## Go library

Add the current development version to your Go module:

```sh
go get github.com/muratmirgun/compact-engine@main
```

Go records the resolved commit as a version. No stable release exists yet.

Import the public packages:

```go
import (
    "github.com/muratmirgun/compact-engine/archive"
    "github.com/muratmirgun/compact-engine/compact"
    "github.com/muratmirgun/compact-engine/jev"
    "github.com/muratmirgun/compact-engine/token"
)
```

Construct `archive.New(directory)`, `token.New("o200k_base")`, and `jev.New(jev.Config{APIKey: key})`.
Pass them to `compact.New(scorer, counter, store)` and handle every constructor error.
Call `defer scorer.Close()` after constructing a Jev client.
Use `compact.NewReplay(scores)` for offline tests.

Run the complete example:

```sh
go test ./compact -run ExampleEngine_Compact -v
```

When embedding `server.Handler`, the caller owns listening, TLS, and shutdown.
The handler cannot determine whether its listener is public; configure authentication explicitly.

# HTTP and transcript API

The API version is `1`. The Go API remains experimental.
The service supports one trust boundary, such as one agent workspace.

## Routes

| Method and path | Result |
|---|---|
| `GET /healthz` | Process liveness; does not check Jev. |
| `POST /v1/compact` | Archive and compact a transcript. |
| `GET /v1/snapshots/{id}` | Return the complete original request. |
| `GET /v1/snapshots/{id}/messages/{message}` | Return one original message. |

When `COMPACT_API_TOKEN` is set, every data route requires this header:

```text
Authorization: Bearer YOUR_SERVICE_TOKEN
```

The service token and the TypeSafe API key are separate credentials.
One service token grants access to every snapshot in that service's archive.

## Compact request

```json
{
  "goal": "Fix cache invalidation",
  "target_tokens": 12000,
  "recent_messages": 4,
  "allow_partial": false,
  "messages": [
    {"id": "u1", "role": "user", "text": "Fix cache invalidation."},
    {
      "id": "a1",
      "role": "assistant",
      "text": "",
      "tool_calls": [
        {"id": "call1", "name": "read_file", "arguments": {"path": "cache.go"}}
      ]
    },
    {"id": "t1", "role": "tool", "tool_call_id": "call1", "text": "File contents..."}
  ]
}
```

| Field | Contract |
|---|---|
| `goal` | Nonblank UTF-8 text, at most 16,000 bytes. |
| `target_tokens` | Positive input budget, excluding tool definitions and reserved output tokens. |
| `messages` | Between 1 and 10,000 messages. |
| `recent_messages` | Defaults to four. Zero disables recency protection. |
| `allow_partial` | Defaults to false. True permits a reduction that still exceeds the budget. |

The HTTP body limit is 8 MiB.
The server rejects unknown fields, invalid UTF-8, and multiple JSON values.

Message IDs must be unique and contain 1–128 ASCII letters, digits, dots, underscores, or hyphens.
Allowed roles are `system`, `developer`, `user`, `assistant`, and `tool`.
Only assistant messages can contain `tool_calls`.
Each tool result must reference an earlier call through `tool_call_id`.
Each call accepts at most one result. Pending calls remain protected.
Tool arguments must contain valid JSON.

Optional protection and dependency fields:

| Field | Meaning |
|---|---|
| `pinned` | Preserve this message and its entire group. |
| `unresolved` | Preserve unresolved work and its entire group. |
| `tool_calls[].side_effect` | Preserve a call with side effects and its group. |
| `group` | Join messages with the same group label. |
| `depends_on` | Join this message with the listed message IDs. |

## Compact response

```json
{
  "version": "1",
  "status": "compacted",
  "applied": true,
  "budget_met": true,
  "snapshot_id": "64-character-sha256",
  "messages": [],
  "decisions": [],
  "scores": {"old-call": {"loss": {"brief": 0.19}}},
  "warnings": [],
  "stats": {
    "input_tokens": 2619,
    "output_tokens": 343,
    "reduction_ratio": 0.869,
    "counter": "o200k_base+canonical-json+8/message",
    "scorer": "jev:jev-latest",
    "scoring_ms": 1025.84,
    "total_ms": 1039.85
  }
}
```

This abbreviated illustration omits messages, decisions, and other proposed action scores.
Actual responses include the complete selected history.

| Status | `applied` | `budget_met` | Caller action |
|---|---:|---:|---|
| `unchanged` | false | true | Use the original history; it already fits. |
| `compacted` | true | true | Use the selected history. |
| `partial` | true | false | Resolve the remaining budget overflow before another model call. |
| `budget_unmet` | false | false | Original history returned; choose another budget or fallback. |
| `scoring_failed` | false | false | Original history returned; handle scorer failure. |

All five states return HTTP 200.
The CLI also returns exit code zero for these valid JSON results.
Check `budget_met`, not just the transport status or process exit code.
`partial` requires `allow_partial: true`.

Each decision contains `group_id`, `message_ids`, `action`, and `reason`.
Scored decisions also contain `score`.
The reason is a fixed engine explanation, not model reasoning.
When no reduction is committed, `decisions` is empty.
The top-level `scores` field still exposes valid scores for diagnosis.

Modern scores contain `loss` entries for every proposed action.
Legacy scores use `relevance` and `detail`.
These values are estimates, not calibrated failure rates.

## Errors

| HTTP status | Meaning |
|---|---|
| 400 | Invalid schema or transcript. |
| 401 | Missing or incorrect service token. |
| 404 | Unknown route, snapshot, or message. |
| 405 | Method not supported on the route. |
| 408 | Request canceled before admission. |
| 413 | Body exceeds the configured limit. |
| 415 | Content type must be `application/json`. |
| 429 | Active request limit reached; no queue is added. |
| 500 | Archive or internal processing error. |
| 504 | Processing canceled or deadline exceeded. |

Handler errors use `{"error":"description"}`.
Unknown routes and unsupported methods use the standard Go HTTP response format.

## Agent integration

```text
request = normalize(goal, history, input_budget)
result = POST /v1/compact(request)

if result.budget_met:
    history = denormalize(result.messages)
    remember(result.snapshot_id)
else:
    handle_budget_or_scoring_failure(result)
```

Expose snapshot recovery as an agent tool when the model needs archived content.
Do not change call IDs or arguments when converting between message formats.
Reject unsupported reasoning blocks, images, or audio instead of silently deleting them.

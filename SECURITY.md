# Security

## Scope

Compact Engine currently serves one trusted agent or workspace.
One service token can access every snapshot in its archive.
Tenant isolation, per-snapshot authorization, encryption, and retention management are not implemented.

The built-in CLI binds to loopback by default.
It requires `COMPACT_API_TOKEN` for a non-loopback address.
Embedded HTTP handlers require the caller to configure their listener and authentication correctly.
Use a TLS reverse proxy for remote connections.

## Data handling

Original transcripts are stored before reduction or external scoring.
New snapshot files use `0600`; newly created archive directories use `0700`.
Existing directory permissions are not tightened.
The archive directory must remain under the control of the service owner.
Checksums detect corruption; they are not encryption or authorization.

Live scoring sends selected conversation context, tool arguments, original previews, and proposed replacements to TypeSafe.
The engine has no automatic secret redaction.
Filter or reject sensitive content before calling it.
The provider key does not belong in transcripts, score files, or published reports.

Scorer output is untrusted input and undergoes structural checks.
Loss scores are not a security guarantee.
Keep critical constraints explicitly protected.

## Reporting a vulnerability

Use [private vulnerability reporting](https://github.com/muratmirgun/compact-engine/security/advisories/new).
Do not put credentials or exploitable private details in a public issue.
If no private channel is visible, open a minimal issue requesting a private contact route.
Describe only the affected component until a private route is available.

The project has no released support window or response-time commitment yet.
Maintainers must confirm that private reporting remains available before each release.

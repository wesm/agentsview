# Evener provider support

Status: revised to follow existing Agentsview provider behavior. Implementation
has not started.

## Outcome and scope

Make Evener a native Agentsview provider. Users can discover, browse, search,
filter, export, and inspect usage for Evener sessions using the same workflows
as other supported agents. Preserve tools, reasoning, session names, model
changes, forks, and subagents rather than importing only visible chat text.

Support local files and Agentsview's existing remote-sync mechanisms. Do not
introduce a direct Evener hub connection, require a running Evener process, or
add an Evener runtime dependency. The integration reads source artifacts and
never writes them.

The supported producer boundary is Evener semantic transcript format v2.
Unsupported versions receive an explicit diagnostic. Legacy-format conversion
is outside this change. Missing metadata is supported because the transcript
is independently meaningful; missing fields must not be invented.

## Approach

Implement an Evener provider and parser behind the existing provider facade,
reusing source discovery, normalized records, sync, storage, and UI machinery.
Keep Evener-specific source topology and decoding in the provider package.
Small local wire structs avoid coupling Agentsview to Evener's dependency tree.

A chat-text import would omit source freshness, relationships, accounting, and
remote companions. A hub API integration would introduce authentication and
availability requirements and is outside the agreed scope. A native file
provider fits the existing architecture and supports offline history.

No archive schema migration is planned. If faithful mapping requires one,
revise this design before expanding the storage interface.

## Producer evidence

The initial source inspection used Evener revision
`f4a3ff2bf889ea4964583b03229cafcde3f7c1c6` and Agentsview revision
`e099ea370cdb9b5cdb79f12543d7381e103499bc`.

Relevant Evener producer files:

- `agent/transcript/transcript.go`: v2 header, entries, framing, and writer.
- `agent/schema/turn.go`: semantic turn kinds and response provenance.
- `llm/types.go`: message parts, tools, media, and usage fields.
- `agent/schema/snapshot.go`: metadata, title, fork, and subagent fields.
- `agent/fork.go`: copied prefix and fork metadata construction.
- `agent/runtime_dir.go` and `cmdutil/statedir.go`: state-root layout.
- `llm/providers/anthropic/response.go`: disjoint input/cache categories and
  separate cache-write lifetimes.

The implementation must add a pinned, reproducible entry to
`docs/internal/session-format-sources.md`, checking the other relevant usage
adapters and writer paths before finalizing accounting fixtures. Source fields
alone are insufficient evidence of when or how a writer populates them.
Fixtures contain synthetic identities, paths, and content, not private logs.

## Discovery, identity, and configuration

Register `evener`, an `evener:` session ID prefix, `evener_dirs`, and
`EVENER_DIR` using the registry's configuration conventions. Respect
`XDG_STATE_HOME` for default discovery, falling back to
`~/.local/state/evener`.

Accept the Evener state root, a project state directory, or a sessions
directory as a configured root. Default discovery traverses
`projects/<project-id>/sessions/`. An explicit custom state directory contains
`sessions/`. Discover only `<session-id>.transcript.jsonl` files and pair each
with `<session-id>.meta.json` when present. API traces and operational logs
must never become additional sessions or duplicate usage sources.

Use the header session ID as identity and validate it against the filename.
Use header working-directory and metadata evidence for project display, not
the opaque project-directory identifier. Apply the existing remote machine
namespace and canonical path rewriting. Deduplicate overlapping configured
roots without merging distinct machine identities.

Session lookup must validate stored-path hints, reject path traversal, and
stay inside the provider's configured topology. Missing roots are harmless;
unreadable roots are errors, not proof that their archived sessions vanished.

## Transcript and metadata mapping

| Source | Agentsview behavior |
| --- | --- |
| Header | Session identity, creation time, initial model, working directory, source build version, and parent hints |
| `USER_INPUT` | User message; contributes to user-message counts |
| `STEERING` | User message only when `steering_source` is `user`; otherwise a system message with its source classification |
| `ASSISTANT` | Assistant text, reasoning, tool calls, response model/provider, and usage |
| `TOOL`, `TOOL_RESULTS` | Tool results linked by call ID, preserving error state and repeated result events |
| `SYSTEM`, `ENVIRONMENT`, `HOOK_COMPLETED`, `ATTENTION_RESOLUTION` | System-classified records with readable content and meaningful event details |
| `CHECKPOINT`, `SUMMARY` | Explicit compaction records; retain the preceding history |
| `MODEL_SWITCH` | Visible model-change record; consume structured switch facts when present |
| `TURN_FAILURE` | Visible diagnostic; do not mark a later successful continuation as failed |
| Metadata | Session name and explicit presence, fork/subagent distinction, and available environment metadata |

Preserve content-part order within the limits of existing normalized text,
thinking, and tool fields. Render structured tool results deterministically,
without losing their call IDs or turning system events into user prompts.
Header system prompts and initial task information remain system context and
must not displace the first human message used for session summaries.

Use existing media representation where available. For media the archive/UI
cannot represent, retain a clear type/name/reference placeholder; never claim
binary fidelity or silently discard the fact an attachment was present. Do not
fetch remote media or arbitrary local paths referenced by a transcript.

Absent sidecars leave transcript-derived fields usable. A sidecar whose ID
conflicts with the transcript must not rename or reparent another session.
Malformed or transiently replaced sidecars trigger retry without erasing
previously valid metadata. Sidecar removal follows the established optional
metadata semantics and never deletes the transcript's archived session.

Use metadata fork information to distinguish forks from spawned subagents.
Header parent/tool hints alone are insufficient: fork creation can copy
subagent header fields. Preserve parent links even when the parent is absent
from the current import. Do not infer successful termination from EOF alone.

## Usage and cost

Report recorded conversation usage, following other providers. Use
assistant-turn usage as the source; do not additionally sum metadata cumulative
totals or API logs. This is not a provider invoice reconciliation feature.
Preserve explicit zero versus absent optional measurements. Keep actual
reasoning counts separate from estimates; reasoning is not an additional
charge on top of output tokens.

Evener normalizes uncached input, cache reads, and cache writes separately.
Map these to Agentsview's normalized usage representation, including distinct
cache-write lifetimes. Context size includes the applicable input/cache
categories. Use each response's recorded model and provider. Fall back to the
header model until a switch; after a switch without structured facts, leave
unattributed usage unpriced rather than extracting a model from display prose.
The separate Evener producer PR adds structured facts to model-switch turns;
use those facts when present. Prices come from Agentsview's existing catalog
and unknown prices remain unknown.

Follow the existing Codex fork policy in `internal/parser/codex.go`: omit a
verified replayed parent prefix from the child's normalized messages and usage,
and preserve the parent relationship. Evener supplies a 1-based
`divergence_turn` over all source entries, so entries before that boundary are
the candidate copied prefix. Verify against the available parent transcript;
do not use parsed-message ordinals or timestamps as the boundary.

When the parent cannot be resolved or the boundary cannot be verified, retain
child content, matching Codex's conservative behavior. Document that shared
history can then contribute usage in both sessions. Do not introduce recursive
ancestry-derived identities, a new billing deduplication scheme, or a separate
accounting-warning subsystem. Spawned subagents without a copied prefix retain
all their own usage. Use existing source-ID deduplication only where the
producer supplies a dependable identity.

Test an ordinary fork, a fork of a fork, missing parents, subagents, and
repeated sync using the established provider test patterns. Backend storage
and export consume the normalized results through existing generic paths.

## Freshness, incremental sync, and reconciliation

Treat transcript plus optional metadata as a composite source. A metadata-only
change must invalidate freshness. Use the existing multi-file stat/hash
capability and companion mechanisms where they fit; do not hide a sidecar
behind a transcript-only size/mtime cache key.

Use the existing streaming discovery and line-reader helpers. Map an ordinary changed
transcript or sidecar directly to its session. Watch the stable state/project
containers needed to discover newly created project and session directories.
Do not scan every session on each file event.

Use existing provider full-parse and append mechanisms. Fall back to a full
parse of the affected session when an append cannot safely preserve model
context, tool pairing, or fork filtering. No new checkpoint or scheduling
framework is needed. Check an active long-session fixture to catch repeated
whole-tree work; optimize only a demonstrated bottleneck. Declare only the
incremental capabilities actually implemented and tested.

Consume newline-complete entries only. Leave a live unfinished final record
for retry, keeping its offset unconsumed. Distinguish that from a malformed
complete record; report corruption and avoid marking the source current or
replacing good archive content with an incomplete parse. Bound record size
with an explicit diagnostic, accounting for Evener's large media records.
Unknown additive fields are tolerated. Preserve an unknown semantic kind as a
system record with its readable message and source kind, without inferring
usage or user-message counts. Use existing malformed-line diagnostics and
retry conventions for invalid records rather than adding a new error UI.

Reconciliation uses provider-owned scopes. Incomplete discovery or parse
failure never authorizes deletion. Source disappearance and tombstones follow
Agentsview's archive retention contract; no destructive archive reset is part
of adding this provider.

## Existing remote mechanisms

Agentsview owns transport, capture, and remote archive replication. HTTP remote
sync is its current path; SSH is documented in `internal/ssh/sync.go` as a
deprecated compatibility transport. Do not build or extend these transports.

Our contribution is the Evener provider: register its identity, source roots,
file selection, and optional metadata companion using existing hooks. Verify
that generic archive export/import preserves its normalized records. Reuse
existing capture plumbing where provider participation is required, excluding
API logs, credentials, and unrelated state. No new remote service, S3 adapter,
or SSH-specific discovery algorithm is part of this change. Capabilities that
need transport work remain unsupported; do not claim support by setting flags.

## UI and documentation

Add Evener to the shared agent label/color catalog and capability-driven
controls. Reuse current transcript, tool, relationship, and usage components.
Update configuration/support documentation and a user-facing changelog entry.
Update every locale if a new localized message is needed.

Verify session listing, search, filters, transcript details, thinking, tools,
parent/child navigation, usage, and export in a browser against scratch data.
Audit hardcoded agent enumerations and capture/remote allowlists so registry
registration does not leave hidden unsupported paths.

## Development isolation

All branch binaries, configuration, databases, caches, fixtures, and logs live
in the isolated worktree's ignored scratch directory or test temporary
directories. Every runtime invocation explicitly selects its scratch
`AGENTSVIEW_DATA_DIR`; verify the resolved config and database paths before
starting sync or serving. Use a separate loopback port and scratch frontend
API target. Do not install the branch binary or change service registration.

Use a fresh config that enables only synthetic/copied Evener roots. Remove
inherited remote mirror, push, telemetry, and service settings from runtime
environments. Do not copy production config, open production databases, run
production migrations, or connect development sync to production destinations.
Tests use `t.TempDir()` and the repository's database helpers. Any PostgreSQL
parity run uses only a dedicated disposable test database.

## Acceptance and delivery

1. Table-driven parser fixtures cover every current semantic kind, structured
   content, metadata conflicts, model changes, tools, and usage presence.
2. Lifecycle tests cover creation, append, partial tail completion, truncation,
   replacement, rename, sidecar-only change, deletion, and retry recovery.
3. Accounting tests cover multiple billing providers, both cache-write
   lifetimes, verified fork prefixes, missing parents, subagents, and repeated
   imports using existing provider conventions.
4. Discovery and changed-path tests compare small and large source trees and
   prove routine event work is bounded to affected sources.
5. Real sync-to-archive tests verify search, export/import, relationships,
   usage, and idempotence. Reuse the existing generic backend and remote
   contracts; add coverage where Evener introduces different normalized data.
   Any backend integration tests use disposable storage only.
6. Run relevant Go checks with CGO and `fts5`, formatting and vet before Go
   commits, frontend checks and tests, and browser acceptance. Record any
   unavailable integration environment accurately.
7. Review the complete diff for correctness, maintainability, privacy, and
   contribution-rule compliance. Run the repository's private-data scrub
   before publication. Include reproducible format provenance.
8. Deliver a focused feature-branch PR with a summary-only description stating
   behavior, scope, and material limits. Do not merge or post issue/PR comments.

The next artifact after approval is an implementation plan identifying the
existing helpers and exact test seams for each step. Execution method is
selected when that plan is reviewed. This design does not authorize replacing
shared storage or remote-sync architecture to accommodate Evener.

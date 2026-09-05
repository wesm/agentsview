# Evener provider implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development for the
> independent parser and provider tasks, with integration and review in the
> coordinating session.

**Goal:** Deliver native Evener v2 support in an upstream Agentsview PR, tested
against isolated storage and reviewed with roborev until no significant issues
remain.

**Architecture:** Decode semantic records into existing ParsedSession and
ParsedMessage types. Register a file provider using existing source-set,
watching, composite-fingerprint, and remote mechanisms. No new transport,
storage schema, or billing subsystem.

**Tech stack:** Go 1.27, SQLite with CGO/fts5, existing Svelte frontend.

**Spec:** ../specs/2026-09-05-evener-support-design.md

## Global constraints

- Work only on the isolated feature branch; never install its binary.
- Runtime data/config and source fixtures must be scratch data. No production
  archive or remote destinations. Use explicit scratch AGENTSVIEW_DATA_DIR.
- Support semantic v2 only; do not delete old archive content.
- Read root and routed agent instructions before edits. Follow existing
  provider behavior; use testify and temporary directories for tests.
- Preserve metadata changes and tools. Costs represent recorded conversation
  usage, not invoice totals. Unknown models remain unpriced.
- Transport ownership stays in Agentsview. Provider hooks may name companions.
- Commit focused changes. Do not merge. Review and fix the complete branch.

## Task 1: Semantic parser

**Files:** Create internal/parser/evener.go and evener_test.go. These files are
owned by the parser implementer; leave provider registration to Task 2.

**Interface:**

```go
func parseEvenerSession(ctx context.Context, path, machine string) (*ParsedSession, []ParsedMessage, error)
```

The path is a local/materialized transcript. The parser reads its optional
sibling .meta.json. Use AgentType("evener") until Task 2 registers AgentEvener.
Return nil session for unsupported pre-v2 input with an explicit error, never
successful empty replacement. Set IsTruncated for an unfinished tail so the
provider can request retry. Complete malformed records return errors.

- [ ] Write tests using a header plus newline-framed entry records; first test
  must fail before implementation. Assert normalized records, not rendered JSON.
- [ ] Decode header/session identity and metadata. Require filename/header ID
  agreement. Derive project from working_dir with existing helpers. Header
  created_at is session creation, entry timestamps are message timestamps.
- [ ] Map every current semantic kind. Human steering counts as user input;
  other steering and diagnostic kinds are system-classified. Preserve unknown
  kinds as readable system messages. Keep reasoning separate from text. Map
  tool calls/results/events, structured results, errors, and media placeholders.
- [ ] Map per-response usage into existing token JSON and aggregate helpers;
  include input, output, reasoning, cache read, 5m write and 1h write. Use
  response model first, then header/structured model_switch timeline. Do not
  parse switch prose; clear fallback after an unstructured switch.
- [ ] Implement Codex-style verified parent-prefix suppression using Evener's
  1-based divergence_turn. Compare source turns, not parsed ordinals. Missing
  or unverifiable parent retains child content. Preserve parent/subagent links.
- [ ] Test same-provider and cross-provider switches; title changes; sidecar
  corruption/identity mismatch; unknown kind; each content type; incomplete
  last line; malformed complete line; absent/zero usage; fork with and without
  parent; nested fork; unrelated equal text must not be suppressed.
- [ ] Run focused tests, go fmt ./..., and go vet ./... before committing.

Test command (with repository build prerequisites restored):

```sh
CGO_ENABLED=1 go test -tags fts5 ./internal/parser -run Evener -count=1
```

## Task 2: Provider, configuration, and lifecycle

**Files:** Create internal/parser/evener_provider.go and provider tests. Modify
internal/parser/types.go, provider.go, and internal/config/config.go/tests only
where registration/default roots require it. Own these files separately from
Task 1.

**Consumes:** Task 1's parseEvenerSession signature. Provider tests can be written
before the parser lands; coordinate compilation rather than introducing stubs.

**Produces:** newEvenerProviderFactory(def AgentDef) ProviderFactory, AgentEvener,
registry entry and configuration defaults.

- [ ] Write a provider lifecycle test that creates an Evener project sessions
  directory and checks discovery, lookup, watch plan and sidecar path mapping.
- [ ] Register evener, evener:, evener_dirs, EVENER_DIR. Respect XDG_STATE_HOME;
  default ~/.local/state/evener, with project/sessions roots accepted explicitly.
- [ ] Use NewSingleFileSourceSet/SourceSetFactory patterns (Reasonix and
  Codebuff). Filter only recognized session layouts and transcript suffixes.
  A changed metadata file maps directly to its own transcript.
- [ ] Fingerprint transcript plus metadata. Ensure metadata removal and
  same-size replacement invalidate freshness; declare required hash semantics.
  Reuse existing stat/hash helpers and source-set replacement/retry behavior.
- [ ] On successful full parse replace normalized messages, so fork boundary
  or title changes cannot leave stale rows. Incomplete tail remains retryable.
  Use safe full-parse fallback; do not invent cursor persistence.
- [ ] Keep remote canonical paths through existing source-set behavior. Add
  only the existing companion/capture hook needed to name the two files; no
  SSH/S3 transport changes. Reject unsafe lookup IDs and unrelated files.
- [ ] Test root variants, overlapping roots, ignored API logs, missing roots,
  new directories, sidecar-only changes/removal, file replacement/truncation,
  missing/corrupt input, repeated discovery, and changed-path work locality.
- [ ] Run relevant parser/config tests, formatting and vet; commit.

## Task 3: Integration, UI, and documentation

**Files:** frontend/src/lib/utils/agents.ts and agents.test.ts;
internal/sync/evener_test.go; docs/configuration.md; README.md; CHANGELOG.md;
docs/internal/session-format-sources.md. Add synthetic testdata if useful.

- [ ] Add Evener label/color following the shared catalog and test lookup.
  Audit agent enumerations and use existing UI controls; no new UI subsystem.
- [ ] Write real sync-to-temporary-archive tests. Discover and ingest a synthetic
  transcript; verify session, messages, tools, usage, title and relationships.
  Re-sync unchanged, append a turn, edit only metadata, then truncate/replace;
  assert actual stored outcomes and no duplicate records.
- [ ] Verify generic export/import and source canonicalization using existing
  test helpers. Do not widen transport scope to satisfy an invented requirement.
- [ ] Record producer sources pinned to inspected Evener commit and structured
  model-switch PR revision. Document v2 floor, roots, fork fallback, usage
  limits, and media placeholders. Use only synthetic names and paths.
- [ ] Run frontend tests and checks, then build a scratch binary. Run sync and
  serve with an explicit scratch configuration enabling only fixture roots.
  Verify in a browser: filter, search, transcript, thinking, tools, usage and
  relationship navigation. Save proof under ignored scratch directories.
- [ ] Commit implementation and documentation changes.

## Task 4: Whole-branch verification and delivery

- [ ] Review task diffs for spec compliance and quality; fix substantive issues.
- [ ] Run full relevant Go tests and repository lint/vet, frontend check/tests,
  and integration acceptance. Dedicated backend services only if needed.
- [ ] Run `roborev review --branch --wait`. Read the actual result; fix
  significant findings, commit and rerun until passing or only minor issues
  remain. Record disposition and evidence for any accepted minor finding.
- [ ] Scrub the complete diff for private data, inspect final branch state and
  create the upstream PR with a summary-only description. No merge.
- [ ] Audit every spec/plan item against actual tests/runtime/review evidence;
  report PR, review verdict, and any material limits accurately.

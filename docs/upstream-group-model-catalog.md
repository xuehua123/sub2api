# 0.2.7-ppx.6 Upstream Group Model Catalog

This change only adds administrator catalog observation and annotations. It does
not change account model mappings, scheduling, pricing, balances or entitlements.

## Model Sources

- Sub2API: authenticated `/model-prices?group_id=...`. The response must echo the
  requested `selected_group_id`; a fallback/default group is rejected.
- NewAPI-compatible providers: authenticated `/api/pricing`, accepting only model
  entries with explicit `enable_groups`. A connection-scoped, versioned cache
  shares this full catalog for five minutes; failed fetches are cached one minute.
  Explicit manual refreshes and background recovery of an active manual batch
  bypass that cache so old entries cannot be reported as freshly observed.
- Otherwise use the isolated `FetchUpstreamGroupSupportedModels` discovery path. Only
  fresh, exact, fixed/inherited bindings with unchanged API-key fingerprints are
  eligible. Dynamic and fallback-chain keys are not attributed to one group.
- Published models and models visible to bound keys are labelled separately.
  Neither is a successful inference probe. Unsupported catalogs and unavailable
  credentials are displayed as unknown/error, not as a successful empty list.
- Validate every page's business status before accepting its models. HTTP 200
  with `success:false`, a nonempty `error`, or an unsuccessful `code` is a failure,
  including when `data` is empty. Invalid entries never silently shrink a list.
- Follow Gemini `nextPageToken` and Anthropic `has_more`/`last_id` on the same
  validated endpoint, retaining auth/proxy settings. All pages must succeed;
  failures, cancellation or repeated/missing cursors retain the previous source
  snapshot. Limits: ten seconds, 100 pages, 8 MiB cumulative response bytes,
  10,000 distinct model IDs and 1 MiB total model-name data per source.
- `upstream` accounts use an in-memory API-key adapter without persistence or
  OAuth refresh. Antigravity passthrough uses its configured Claude-compatible
  gateway and dual auth headers, not the official Cloud Code OAuth endpoint.
- Each round selects at most eight sources, prioritizing missing/changed and then
  least-recently-attempted accounts. Remaining sources continue in later rounds;
  there is no permanent first-eight cutoff. Each account call has a ten-second
  timeout; management discovery has a separate fifteen-second budget.

## Persistence And Concurrency

- Migration 242 is additive: an independent snapshot table and an exclusion array
  on annotations. Existing application code can run with these additions.
- Migration 243 adds per-account snapshots and a current-source validation view.
  Fingerprints cover credentials, request configuration, proxy endpoint and group
  identity; they exclude usage, rate, balance, quota and observation timestamps.
- Migration 244 separates `identity_valid` from fetch `eligible`. A temporary
  binding-probe error or expired observation prevents new fetches but keeps the
  last successful models/tags visible as partial/stale. Changed credentials,
  endpoint, group identity or removed accounts still invalidate results immediately.
- Keys are `(connection_id, remote_key)`, independent of regenerated group IDs.
  Snapshots from a previous connection configuration version are hidden.
- A per-group database lease coordinates application instances. Manual refresh
  has a 30-second cooldown; each process permits at most two active refreshes.
- Migration 245 adds persistent refresh generations. Manual refresh starts a
  batch spanning all eligible sources, even those with fresh cached results.
  Repeated clicks resume the active batch rather than starting it again. Each
  source is attempted once in that batch; failures keep their old models but are
  not counted as successful. Remaining sources continue in the background even
  when automatic periodic sync is disabled, without enabling periodic sync.
  A crashed worker can be replaced after its lease expires and the 30-second
  cooldown passes, even when an earlier published result has a six-hour TTL.
- The independent model worker checks two due groups per 30-second tick under its
  own distributed leader lock. It does not occupy the wallet/rate worker.
- Successful source snapshots expire in six hours; failed sources back off from
  five minutes to one hour. Each success replaces that source's result, including
  explicit empty lists; each failure retains only that same source's old result.
  Partial runs therefore publish newly observed models without losing results
  for other sources. Invalidated sources are excluded immediately on both reads.
- Saving validates the connection version and group existence. Account-backed
  results additionally recheck current bindings and configuration fingerprints.
  Changed/new sources become due immediately, independently of the group TTL.
  Requests do not hold database transactions while contacting upstream servers.
- Management catalog requests do not follow redirects. Secrets and raw upstream
  errors are not included in model responses.

## Tags And UI

- Deterministic model-ID rules generate family and capability tags. Unknown IDs
  are not guessed. Image input is not classified as image generation.
- Effective tags are manual additions plus automatic tags minus explicit
  exclusions. Removing an automatic tag suppresses it on subsequent refreshes;
  restoring automatic categories clears exclusions but retains manual additions.
- Catalog queries support model-ID search and effective-tag filtering. List pages
  return counts and at most three preview model IDs per group. Full IDs load only
  when opening a group, with at most 50 rendered rows at a time.
- Website links derive from the configured management URL, preserving deployment
  path prefixes while stripping common API suffixes, queries and fragments.
  Non-HTTP links and URLs containing credentials are rejected.
- The layout retains the application sidebar, existing colors and table style.
  Narrow screens scroll within the table rather than overflowing the page.
- While an open, visible dialog has `refresh_in_progress`, it reads progress
  every five seconds, with no overlapping requests or upstream-triggering POSTs.
  It stops on completion/close/unmount, pauses when hidden, and retries transient
  read errors with exponential backoff before offering a read-only retry button.

## Local Verification

Regression coverage includes classification, cross-group response rejection,
shared catalog caching, redirect rejection, ambiguous/stale/changed bindings,
lease coalescing, failed/partial snapshot preservation, annotation overrides,
group rediscovery, model-search pagination and bounded list payloads. UI tests
cover website safety, on-demand loading, search, selected refreshes, stale
response suppression and rendering bounds.

The local visual preview uses explicitly labelled synthetic data. It does not
call production upstreams or change production records.

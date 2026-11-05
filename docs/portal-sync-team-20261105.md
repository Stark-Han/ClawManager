# Team portal analytics synchronization — 2026-11-05

## Source and preservation boundary

Implementation target is the TEAM checkout `codex/team-personal-integration`,
starting at `9e8e1b7f`. The PERSONAL checkout is read-only: selected committed
portal functionality was ported from `codex/openclaw-102-team-integration`
at `3a607442`, not merged wholesale and not taken from its dirty working tree.
No personal upgrade/restart controller, production configuration, historical SQL,
Runtime source or Worker tool contract was changed.

The Team tab, owner isolation, templates, six-member layout, internal scrolling,
member links opening new tabs, file search/edit/preview, ordinary instance Pro
access and the Lite-only new-Team admission policy remain in place. Dispatch,
serial execution, work-item identities/revisions, dependencies and formal Leader
completion retain their existing implementation.

## Adapted capabilities

- Background statistics use bounded, independently checkpointed source/history/
  recent/repair/projection/publish/ranking/report phases. Source failures do not
  cancel later phases; activity cancelled before persistence remains buffered.
- Active/quiet history selection, backoff, generation fences, cross-day repairs,
  isolated readers and diagnostics reuse the existing owner records table.
  Readers must be reaped before their guard is released; an uncertain exit is
  not treated as a completed read or permission to launch unlimited replacements.
- OpenClaw 7.1 JSONL remains supported; 10.2 SQLite, compressed events and cold
  archives have bounded read-only adapters. Unknown formats, corruption and
  unsafe network-filesystem WAL reads remain failures rather than measured zero.
- Team's OpenCode 2.0.27 differs from the personal source: it materializes native
  input/results in `session_v2`/`session_message`. A separate read adapter selects
  that format when present and retains the old format otherwise. It excludes
  child sessions, tagged Team automation, synthetic continuations, failed replies
  and tool-only output. Repeated successful replies count one human input.
- Calendar-aware visits, heatmap sizing, snapshot/batch timestamps and bounded
  read-failure notices are merged into the Team Dashboard composition. Visits
  are added to a request-owned copy, never to a shared cached snapshot.
- Owner session invalidation covers the separate Team client and pages. An
  upstream/provider 401, permission 403 or temporary 503 does not become an owner
  logout. Queued responses cannot restore invalidated owner content.
- The instance workspace can collapse without destroying its Runtime frame.
  Connection recovery only refreshes access/cookies, is bounded and origin/
  frame checked; it does not restart a Runtime or model execution.
- Administrator-only northbound portal controls now expose saved user statistics,
  bounded sync diagnostics, safe next-batch retry and versioned calendar preview/
  publish/rollback. Merely querying these records does not trigger native scans.
  Existing Team settings and scopes were retained during manual composition.

## Persistence and rollback

No schema migration was added or renamed. Personal `063` and Team `067` owner
records migrations have the same table contract; Team keeps its own migration
identity. No epoch, XP, reward, owner, task, instance, binding or workspace reset
is part of this change. The live preflight found historical reader guards idle.

New control/diagnostic/calendar records use the existing versioned records
envelope. Legacy readers select explicit record types and ignore those extra
types. A rollback must stop the new App and confirm its reader processes have
exited before starting the prior App; never delete those records, clear a held
guard on a timer or reset statistics as a rollback shortcut. Existing owner facts
and unique evidence keys remain authoritative. Use the retained prior deployment
spec and image, not a broad Runtime manifest.

## Necessary verification

- Before implementation, a cancelled activity-flush test reproduced discarded
  uncommitted activity; it passes after the fix.
- Targeted real MySQL checks cover preserved rewards/facts, old generation
  rejection, bounded repair/replay, source failure isolation, restart/checkpoints,
  safe diagnostics and global saved-statistics pagination.
- Linux image tests exercise the actual isolated reader and SQLite CLI,
  OpenClaw 10.2 retired JSONL/WAL handling and the new OpenCode v2 human/automation
  separation, including unchanged database bytes after reading.
- Frontend production build, owner failure classification, existing Team
  creation/ordinary-Pro checks and a local browser fixture of the compiled UI.
  The fixture covers four Dashboard tabs, Team-only owners, six member links,
  wide/short/mobile containment, workspace collapse, retained transient-failure
  snapshots and session invalidation. It is not a live model conversation or
  authenticated live-browser visual acceptance.
- Windows service tests depend on Unix paths/hardlink semantics; acceptance uses
  Linux for the full backend suite, with the sibling Runtime, repository assets
  and Node available. Initial fixture/dependency failures are retained in local
  logs and are not reported as passes.

Task-owned evidence is kept outside Git in the local ignored diagnostic directory.
No credentials, cookies, signed links or private transcripts are included here.

## Candidate and release record

Candidate: `10.130.14.23:5000/clawmanager-hxc-app:team-portal-sync-20261105-v1`.
Application source: `c2f9240ec55fe454883a7dec76548d0f6e008703`; subsequent
documentation/browser-fixture changes do not alter the application binaries.

The image reuses the accepted prior image by immutable digest for unchanged
entrypoint, nginx and Hermes UI layers; all four Go binaries and the built portal
assets are replaced from this Team checkout. The standard repository Dockerfile
also contains the required SQLite dependency and builds the new source normally.
The exact candidate starts against a disposable MySQL, serves the expected
version and matching frontend, passes nginx validation and preserves `no-store`
on the SPA entry. The isolated-reader command exits before normal server setup.

At this documentation checkpoint, registry publication and 32543 post-deployment
acceptance are pending. Append their actual digest, observations and limitations
after execution; image build or Pod readiness alone is not acceptance.

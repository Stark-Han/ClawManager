# Team lifecycle and upstream integration — 2026-11-10

## Sources and merge method

Team repositories only, branch `codex/team-personal-integration`.
App baseline `dc805c22`, upstream `cd7b74ab`; Runtime baseline `8b8ed42`,
upstream `4510b71`. Runtime merge commit `0e1a526`.

Resolve individual conflicts rather than replace whole files. Preserve the local
owner-history scheduler, isolated reader, OpenClaw cold archive acknowledgements,
DSH cursors, Team-compatible placement and shared observation cache. Add upstream
credential-scoped NFS reads underneath that reader and per-slot readiness inside
the existing identity checks. Keep all auxiliary CLI entrypoints before startup.
Preserve native payloads and Team adapters when rebuilding the Runtime managers.

## Behaviour

Ordinary Lite instances retain upstream idle policy. Team members never enter
the individual idle candidate scan. The Team uses the same settings and idle
duration, but stops only as a group after the latest real user task formally
succeeded and no task, work item, delivery, model request, command or write is
outstanding. Bootstrap introductions are not user-task completion. Unknown
business states block automatic stop. Agent presence, LastSeenAt, heartbeat,
member updated_at, GET requests and UI polling never extend business idle time.
Existing Teams receive a fresh reliable-observation grace period.

Migration101 adds a separate runtime lifecycle row; existing Team business status
and task scheduling contracts remain unchanged. Admission and model requests
share the lifecycle row lock. The controller freezes original instance IDs,
generations and workspaces, reuses the upstream per-instance idle request,
teardown proof and slot release, and reports partial outcomes honestly.
An unstarted stop with a failed preflight can safely release its admission gate;
an uncertain external stop retains protection. Start recovers original members,
workers before Leader, checks runtime/collaboration readiness, and sends no task.
Retry of a failed operation retains its identity. No new Worker tool parameters.

CM and northbound expose `POST /teams/:id/start` and `/stop` under their existing
authentication paths. Northbound requires `teams:manage` and owner authorization.
Stopped cards show Start Team; member state is intentionally stopped rather
than a runtime fault. History/files remain readable and message drafts survive.
Direct member lifecycle APIs cannot bypass Team management.

The lifecycle row is registered as backup metadata/reference-only: copied
operations must not replay in another installation. Actual Team/business data
and ordinary instance Pro capability retain their prior policies.

## Validation and operational limits

Linux Runtime upgrade/gateway/slots/workspace tests passed. Real disposable
MySQL tests cover whole-Team admission, fresh-heartbeat irrelevance, unfinished
requests/reviews/deliveries, last-user-task completion, duplicate requests,
task/stop races, cold adoption and partial failure. Ordinary idle controller
failure recovery/data-retention tests run for all four Lite types. Fresh embedded
SQL migrations pass the system-backup schema contract check. Frontend build and
backend regression are required before publication.

The local Docker kernel reports Landlock ABI1. Two upstream private-history
Landlock tests require ABI3 and are explicitly excluded locally; this is not
evidence that the optional NFS profile passed. Upstream NFS configuration,
defaults and security prerequisites are unchanged. Existing deployment profile
must not be switched merely by merging or updating these images.

Release must retain immutable pre-release images/configuration and a private
database snapshot. Do not force-restart occupied pools. Use new compatible pools
or the existing confirmed-stop maintenance path for idle acceptance members.
Do not clear bindings, pending stop records, writer fences or task status to
make acceptance pass. Runtime images and server-side acceptance results are
recorded after actual publication. No minute-scale test timing in release images.

## Rollback

### Rootless node prerequisites and manual failed-start recovery

Ubuntu restricted user namespaces can still deny NET_ADMIN inside bubblewrap
even when the outer container uses AppArmor Unconfined. A no-business-volume
probe using the exact upstream `TestRootlessNetworkReal` reproduced that denial
on node1. An explicitly named HXC-only Localhost profile granting `userns` made
that test pass (network, relay and cleanup). The global sysctl remains enabled.
`RUNTIME_SLOT_APPARMOR_PROFILE` opts slots into this provisioned profile; empty
retains upstream defaults and the coordinator is not changed. Provision the
profile on every eligible node before enabling it. A missing profile must fail.

A failed old namespace preparation can retain unconfirmed cleanup indefinitely.
Normal automatic/graceful stops still require the native C1 commitment plus
exact kubelet termination, writer-lock reconciliation and external-write fences.
For an explicit administrator recovery only, an immutable ConfigMap named
`runtime-slot-fence-<existing-stop-operation>` can request replacement of one
exact old container by an immutable image. Its request schema is defined by
`runtimeSlotFenceRequest`; it records the complete existing non-graceful intent,
old ContainerID and replacement image before any external replacement.

That request is not proof. The controller accepts this separate evidence source
only after kubelet reports that exact container terminated after request creation
and a different container is running in the same Pod UID/slot. It also verifies
closed PID namespaces, no host namespace/mount, no privilege escalation and only
SETUID/SETGID in all serving containers. The persisted proof identifies its
source (`kubelet-container-fence-v1`) and request UID; it never fabricates a native
C1 receipt or claims graceful shutdown. The new worker still acquires the old
NFS lock, reconciles the same assignment and waits for controller finalization.
Keep the request and proof for audit. Unknown/changed identities remain blocked.
No owner/model endpoint creates this operator request; no automatic container
replacement or Task/Worker API changes were added.

Focused backend tests cover real pipeline dispatch of this proof and reject
wrong CID/Pod/operation, mutable or late request, live old container, altered
image, host/shared namespaces, host mount and extra outer capabilities. Runtime
tests exercise both proof sources through writer-lock refusal, idempotent
reconciliation and finalization. Live four-member task acceptance is still pending.

Complete or reconcile pending operations first. Retain schema and native stop
proofs. Use a compatible App that understands the lifecycle row; blindly running
an older Team monitor can misclassify intentionally stopped members. Retain
native images and persistent data, and follow upstream confirmed-stop rules for
any pool/storage rollback. Disabling ordinary automatic stop does not remove
the need to reconcile already-issued member stop operations.

# Team lifecycle repair: timed follow-up, 2026-11-10

## Scope and source

Team repositories only, branch `codex/team-personal-integration`.
App: `D:/test/ClawManager-team-upstream-merge`; Runtime: `D:/test/AgentsRuntime`.
Personal repositories were read-only. Existing Team dispatch, serial execution,
dependencies, explicit receipts and Leader closure are unchanged. No Worker
required parameters were added. This is **not a completed Team acceptance**.

## Completed changes and evidence

* Runtime `ab1f414`: management verifies the real authorized Team group and pins
  path/device/inode across the helper's user namespace. The helper rechecks that
  exact directory rather than trusting an unmapped GID. Replaced directories,
  other Team paths/groups and missing proofs are rejected. NFS config writers
  preserve management-owned shared metadata instead of trying to chgrp it.
* Runtime `2013b58`: Hermes Desktop Web's reviewed Team environment omitted
  `CLAWMANAGER_TEAM_SHARED_GID`. Therefore actual native commands received no
  supplemental group: preparation could read the shared directory but config
  writes failed with EACCES. Forward this existing field; existing sandbox
  admission still validates it against the actual authorized directory.
* App `d923625c`: durable Lite restart reuses `instance_idle_stop_operations`
  and the existing controller. Keep the old generation until confirmed teardown,
  persist a restart follow-up, start once, and recover lost responses by observing
  the next generation. Explicit Stop cancels unstarted follow-ups; stale scans
  reload the operation. Start refusal is blocked rather than replayed forever.
  CM reports pending shutdown as accepted; northbound keeps its operation pending.
  Normal Pro/legacy paths retain their existing implementation.
* Real disposable MySQL tests passed: original source fencing, failed teardown,
  controller handoff, one new generation, lost final progress write, cancellation
  and stale snapshots. Affected services/repository/handlers/northbound tests pass.
* Linux gateway/slots/managedconfig/app checks and affected Hermes/OpenClaw/
  OpenCode/DSH Team tests pass. Rootless namespace tests ran on the HXC server:
  no proof rejects the unmapped group; valid pinned proof permits config writing.
* Real server permission probe: shared inode114442151 has GID1000 outside and
  GID65534 inside the child namespace. Supplemental group1000 permits temporary
  create/write/rename; clearing groups reproduces EACCES. Probe files were removed.
* Ordinary non-Team Hermes608 restarted on32543: old generation1 restart intent
  completed, generation2 binding `gw-608-2` running. A direct authenticated Unix
  slot-state query confirmed the native gateway itself running. Workspace remains
  `/workspaces/hermes/user-1/instance-608`.
* Original Team182 task395 report remains3541bytes, SHA256
  `3309f56d5edf73193021c62ce9e2685ca51d55c8f3f844497cd16984ad72cd17`.

## Deployment and operational findings

App v13 (`d923625c`) is deployed; Core and northbound Gateway are Ready. The exact
`/home/hxc/clawmanager-apply.sh` invocation with APP_IMAGE v13,
TENANT_SUFFIX=-hxc-peer and NODE_PORT32543 completed with exit0.
App digest: `8d53d1c79b2146f2c91e6e7299d408adc7bbb6d3c845b154c1bf802bed256078`.

OpenClaw/OpenCode/DSH v8 protected pools are Ready. Hermes v10 from2013b58 was
published and its previously occupied pool was updated only after ordinary608's
real Stop released the binding. Hermes v10 digest:
`12fca8230f5a98ae609da404573a12a6edd44d7b9e6e93427c49851ebfe4e3ff`.
Check the final status appended below before deciding what is actually serving.

One OpenCode replacement landed on k8s-master, whose kubelet rejects HostUsers
because UserNamespacesSupport is disabled. The owned OpenCode pool was constrained
to verified node1. No cluster-wide feature gate or security default was changed.
Retain this node eligibility requirement for future pool reconciliation.

The first Hermes v8 packaging was overwritten by a concurrent Pro-agent build;
it failed startup, while the old healthy pool remained. Corrected Lite entrypoint
is in v9. App v12 was dynamically linked for an Alpine base and could not launch;
v13 is CGO_ENABLED=0 and its executable was launch-checked in the final image.
Do not use v8 Hermes or App v12. Superseded apply was terminated; final v13 apply
was rerun successfully. Do not run competing apply commands in future releases.

The catalog Pro-preservation assertion initially used only id as identity; built-in
default rows share id0. Corrected comparison uses id/type/backend/variant and
passes. Only four Lite catalog entries were updated; ordinary serving pools and
Pro catalog entries were retained.

## Still required; execute once, in this order

1. Verify Hermes v10 is Ready and the correct Lite entrypoint/digest is serving.
   Ordinary608 was stopped using the real API to permit the unbound pool update;
   its workspace remains. Do not patch SQL state or force a bound Pod.
2. Confirm the pool, App approved-image pin, site overlay and Lite catalog use
   the same immutable Hermes digest. Preserve the other three pins and Pro rows.
3. Retry the existing Team182 start operation, retaining IDs603/604/605/606 and
   its maintenance marker. Require native and Team readiness for all four. Do not
   release maintenance or dispatch a task merely because Pods are Ready.
4. After all members are ready, release only the migration's own maintenance
   marker by compare-and-delete. Submit one small four-runtime real task using
   existing tools; inspect work-item count, explicit receipts, Leader final
   completion and actual final file. No extra Worker fields or forced success.
5. Complete whole-Team Stop -> confirmed4/4 stopped -> Start original4/4 ready.
   Recheck original task395 report and new task history/files. Existing MySQL
   tests cover idle boundary/heartbeats; do not wait a real day or shorten the
   release timeout for acceptance.
6. Record exact IDs, native observations, operation results, commits and image
   digests. Update this document and the detailed lifecycle report. Only then
   mark the feature fully accepted. No fresh task has passed in this follow-up.

If a new failure appears, run the exact failing helper entry with matching
identity/groups/mounts against disposable copies before building another image.
The earlier credential-only copy test and owner-group fixture did not reproduce
the full Hermes command environment; do not repeat those as sufficient evidence.

## Retrospective and rollback

The initial estimate missed two independent group contracts: namespace-visible
identity and forwarding of the existing supplemental-group field. Packaging
mistakes and overlapping release attempts then consumed avoidable time. Future
candidate builds must use unique output paths, explicit Lite entrypoints and
CGO settings, and launch checks in the actual final base before pushing. Resolve
one precise reproduction first; serialize deployment and verify readiness before
starting acceptance. Reuse passed checks, not their incomplete fixtures.

Private snapshots, migration journals, original image/configuration snapshots and
native stop proofs remain under `/home/hxc/team-lifecycle-20261110/`. Keep schema
and persisted lifecycle metadata. Do not roll back to code unaware of intentional
Team stops, delete bindings/proofs, or restart old tasks. Keep maintenance while
readiness is unconfirmed. An image rollback must still use confirmed-stop rules;
it must not revert live database/workspace contents.

## Final observation before17:40 deadline

At17:38-17:39 CST, App/Core and northbound Gateway v13 and all four Runtime
deployments were Ready; final v13 apply exit0 was read back. Hermes v10 digest
above is actually serving and all four Lite catalogs are synchronized. Pro
catalog preservation passes with the corrected complete row identity.
Ordinary608's real Stop completed and its binding is absent, data retained.

The original Team182 start was retried only after Hermes/Core rollout readiness.
Hermes604 generation12 reached native `running` (`gw-604-12`), confirmed through
the authenticated Unix slot-state endpoint. This proves the previously failing
directory preparation and config write passed in the real protected instance.
The Team is still being observed for whole-group readiness. Maintenance remains;
no new model task was submitted. The four-member task/Stop/Start acceptance is
not complete. Do not describe these changes as fully accepted or remove this
remaining requirement after seeing a successful image push or one member start.

At17:39:25 CST, owner API reports Team starting1/4: Hermes is idle/online;
OpenCode605 generation2 failed workspace preparation. Native log at09:39:08 UTC:
default project `starter` stat denied, followed by `workspace_helper_failed`.
Exact failing sub-step is not yet proven. Leader603 and DSH606 have not started
in this latest attempt. Next implementation must first reproduce OpenCode's
actual helper command with its profile-generated environment, supplemental
groups and legacy `starter` directory metadata; do not repeat the passed Hermes
credential/namespace work. Verify equivalent group forwarding across all four
profiles before the next consolidated candidate. Then perform the remaining
four-member task/Stop/Start closure above. Maintenance is deliberately retained.

# Workbuddy Pro Windows Runtime Progress

> 2026-08-11 update: Workbuddy Pro now supports an explicit `linux` or `windows`
> runtime variant from the same system-image card. This document keeps the
> Windows VM implementation and validation history below. The Linux variant
> uses Webtop on port `3001`, mounts its PVC at `/config`, and exposes the
> workspace browser; the Windows variant continues to use port `8006` and
> `/storage`, without Linux-side workspace browsing. Codex Pro follows the same
> variant contract.

## Scope

Workbuddy is a Pro-only Windows desktop runtime backed by `dockur/windows`. Each instance uses a dedicated Deployment, Service, and Longhorn PVC cloned from a prepared golden PVC.

## Implemented

- Uses the Windows runtime image on port `8006`, with the VM disk mounted at `/storage`.
- Clones an immutable, bound golden PVC named by `CLAWMANAGER_WORKBUDDY_GOLDEN_PVC` into the user's namespace before creating the Deployment.
- Requires Pro mode, at least 6 CPU cores, at least 12 GiB container memory, and an 80 GiB PVC. The guest disk remains fixed at 64 GiB.
- Reserves 2 GiB container memory for QEMU and runtime processes, then assigns the remainder to the Windows guest.
- Runs privileged for KVM and TUN access, probes Windows RDP on `3389`, and allows 120 seconds for clean shutdown.
- Schedules only on nodes labeled `clawmanager.io/windows-runtime=true`; those nodes must provide KVM and TUN devices.
- Proxies the browser desktop over HTTP on `8006` and publishes TCP `3389` on the instance Service.
- Removes Linux Webtop, runtime Agent, workspace browser, gateway injection, and skill-scan assumptions from the Windows MVP.
- Adds a migration from the former Linux Workbuddy image default to the Windows runtime image.
- Persists the actual instance PVC name so an instance can claim a prewarmed clone instead of using only the legacy ID-derived name.
- Maintains a leader-controlled pool of cloned, attached, fully read PVCs. Pool size and holder image are configured by `CLAWMANAGER_WORKBUDDY_PREWARM_POOL_SIZE` and `CLAWMANAGER_WORKBUDDY_PREWARM_IMAGE`.
- Reads CPU and memory use from the Kubernetes Metrics API and merges it into the existing runtime details response.

## Golden Image Contract

The golden PVC and clone PVC must be in the same namespace, use the same CSI storage class, and request exactly the same capacity. For the first validation, prepare `workbuddy-golden-v1` in `clawmanager-ltt-user-1` with a cleanly shut down `/storage/data.qcow2` containing Windows, Edge, and Workbuddy.

This namespace restriction means each user namespace needs its own prepared golden PVC until a cross-namespace seed is implemented. The prewarm controller automatically maintains a pool only in namespaces where that golden PVC exists.

## Remote Validation (2026-08-06)

The first end-to-end validation ran in `clawmanager-ltt-system` and created instance `1019` through the authenticated ClawManager API.

- Runtime image: `10.130.14.23:5000/windows-vm-workbuddy:2026.8.6-mvp2`
- Golden PVC: `clawmanager-ltt-user-1/workbuddy-golden-v1`
- ClawManager image: `10.130.14.23:5000/clawmanager-ltt:2026.8.6-workbuddy-windows-mvp2`
- API create response: 0.2 seconds
- PVC became Bound: about 2 seconds
- Standalone warm clone smoke test reached RDP readiness: 84.5 seconds
- Full ClawManager create reached RDP readiness: 398 seconds
- Full create storage clone/attach phase: 383 seconds
- Windows boot after the container started: 15 seconds
- Browser desktop through the ClawManager proxy: HTTP 200 with the noVNC page loaded
- Graceful Windows shutdown during smoke cleanup: 11.9 seconds

The full-create variance came from Longhorn copying and rebuilding a two-replica clone (about 24 GiB of allocated blocks), not from Windows startup. The PVC reports Bound before that backend copy is ready for a workload, so Bound time alone is not a useful readiness estimate.

The prewarm iteration was then deployed with a target pool size of two:

- Metrics Server `v0.8.1` installed and the Metrics API verified available.
- Final ClawManager image: `10.130.14.23:5000/clawmanager-ltt:2026.8.6-workbuddy-prewarm2`.
- Existing instance `1019` increased to 6 CPU cores and 12 GiB container memory; the Windows guest receives 10 GiB.
- Prewarm pool startup produced two attached, fully read, Ready PVCs in about five minutes as a background cost.
- Instance `1021` was created through the authenticated ClawManager API and claimed `workbuddy-prewarm-nthh927w`.
- API create response: 0.78 seconds.
- API create to RDP readiness: 17.17 seconds, down from 398 seconds (about 96% faster).
- The controller immediately started a replacement clone after the claim.
- Browser noVNC and native RDP were both verified after creation.
- Runtime details returned live Kubernetes usage for both Windows instances; instance `1019` sampled at 17.2% of its 6-core limit and 10.1/12 GiB container memory.

Longhorn may take several additional minutes to restore the replacement clone to two-replica HA after a claim. This happens in the background while the remaining Ready pool item continues to serve fast creates.

The attempted QEMU `lossy=on` VNC mode was removed because it prevented the WebSocket listener from becoming available with the current runtime image. Browser noVNC remains compatible, while native RDP is the preferred low-latency desktop path.

## Deferred

Guest Agent installation, managed file browsing, skill synchronization, Windows application lifecycle management, cross-namespace golden distribution, direct Nginx desktop proxying, and warm VM pools are intentionally deferred until the desktop creation path is stable. The implemented pool warms disks, not running Windows VMs.

# Progress

## 2026-08-25

- Added a dedicated `workbuddy-linux` pod security mode with privileged execution, privilege escalation, `NET_ADMIN`/`SYS_ADMIN`, and unconfined Seccomp/AppArmor. This elevated mode is limited to Linux WorkBuddy instances.
- Kept Windows WorkBuddy on its existing KVM privileged path and left other runtime security modes unchanged.
- Preserved `runtime_variant` when saving system image settings and temporarily hid only the Windows WorkBuddy image card. Existing Windows settings remain stored for later restoration.
- Limited the instance workspace panel to Linux WorkBuddy and compatible legacy `/config` records.
- Verified the complete Go test suite, focused frontend source contracts, and the production frontend build. Repository-wide frontend lint remains blocked by pre-existing findings outside this change.

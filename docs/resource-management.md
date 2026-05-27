# Resource Management Guide

Resource Management is the reusable asset layer for OpenClaw workspaces in ClawManager. It is centered on channels, skills, bundles, and the snapshots used to compile those assets into instance-ready configuration.

## Main Resource Types

- `Channels` for workspace connectivity and integration templates
- `Skills` for reusable uploaded packages that can be installed into runtime instances
- Config skills for bootstrap configuration delivered through runtime environment payloads
- `Bundles` for composing repeatable resource sets, including both config resources and uploaded skills
- injection snapshots for tracking the compiled result applied to an instance

## Core Workflows

1. Create or import channels and skills in the OpenClaw Config Center.
2. Organize selected config resources and uploaded skills into reusable bundles.
3. Review scan posture for skills through Security Center.
4. Apply resources or bundles to OpenClaw workspaces.
5. Inspect runtime state and instance-level resource results after injection.

## How It Connects to the Platform

- Resource Management defines what should be delivered to a workspace.
- Config resources are compiled into bootstrap environment payloads. Uploaded skills in a bundle are installed through the Agent Control Plane skill installation path.
- Agent Control Plane applies and tracks those changes at runtime.
- Security Center and `skill-scanner` help review the risk posture of reusable skills before broad rollout.

## Related Guides

- [Security / Skill Scanner Guide](./security-skill-scanner.md)
- [Agent Control Plane Guide](./agent-control-plane.md)
- [Admin and User Guide](./admin-user-guide.md)

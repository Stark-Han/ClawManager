import type { AgencyAgentProfileKey } from "./agencyAgentProfiles";

export type RuntimeType = "openclaw" | "hermes";
export type ResourcePresetKey = "small" | "medium" | "large" | "custom";

export type TeamMemberTemplateMember = {
  memberId: string;
  name: string;
  role: string;
  runtimeType: RuntimeType;
  description: string;
  resourcePreset: ResourcePresetKey;
  isLeader: boolean;
  cpuCores: number;
  memoryGb: number;
  diskGb: number;
  gpuEnabled: boolean;
  gpuCount: number;
  image: string;
  agentProfileKey?: AgencyAgentProfileKey;
};

export type TeamMemberTemplate = {
  id: string;
  name: string;
  teamName?: string;
  description?: string;
  source: "builtin" | "custom";
  members: TeamMemberTemplateMember[];
};

const baseMember = (
  overrides: Partial<TeamMemberTemplateMember>,
): TeamMemberTemplateMember => ({
  memberId: "worker",
  name: "",
  role: "developer",
  runtimeType: "openclaw",
  description: "",
  resourcePreset: "small",
  isLeader: false,
  cpuCores: 2,
  memoryGb: 4,
  diskGb: 20,
  gpuEnabled: false,
  gpuCount: 0,
  image: "",
  ...overrides,
});

export const BUILTIN_MEMBER_TEMPLATES: TeamMemberTemplate[] = [
  {
    id: "builtin-leader-worker",
    name: "Standard Two-Member Team",
    teamName: "research-team",
    description:
      "Leader-mediated Team: the Leader decomposes goals, coordinates members, and integrates results; the Worker executes implementation tasks and reports progress.",
    source: "builtin",
    members: [
      baseMember({
        memberId: "leader",
        name: "team-leader",
        role: "leader",
        description:
          "Team Leader / Agents Orchestrator: decomposes goals, coordinates members, maintains context, validates member outputs, and reports externally.",
        resourcePreset: "small",
        isLeader: true,
        agentProfileKey: "agency.agents-orchestrator",
      }),
      baseMember({
        memberId: "worker",
        name: "team-worker",
        role: "developer",
        description:
          "Senior Developer: executes implementation tasks assigned by the Leader, reports progress, lists changes, and escalates blockers.",
        agentProfileKey: "agency.senior-developer",
      }),
    ],
  },
  {
    id: "builtin-dev-qa-docs",
    name: "Delivery Three-Member Team",
    teamName: "delivery-team",
    description:
      "Delivery Team: the Leader decomposes and coordinates work, the Developer implements and integrates, and the Reviewer verifies tests, regressions, and delivery quality.",
    source: "builtin",
    members: [
      baseMember({
        memberId: "leader",
        name: "delivery-lead",
        role: "leader",
        description:
          "Agents Orchestrator: decomposes requirements, sets priorities, dispatches tasks, manages risks, and integrates results.",
        resourcePreset: "medium",
        isLeader: true,
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.agents-orchestrator",
      }),
      baseMember({
        memberId: "developer",
        name: "developer",
        role: "developer",
        description:
          "Senior Developer: implements code, integrates interfaces, adds necessary tests, and provides reproducible delivery notes.",
        resourcePreset: "medium",
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.senior-developer",
      }),
      baseMember({
        memberId: "reviewer",
        name: "reviewer",
        role: "reviewer",
        description:
          "Evidence Collector / Reviewer: verifies behavior, checks regressions, gathers evidence, reviews delivery items, and gives a PASS/FAIL verdict.",
        agentProfileKey: "agency.evidence-collector",
      }),
    ],
  },
  {
    id: "builtin-software-engineering-team",
    name: "Software-Engineering-Team",
    teamName: "software-engineering-team",
    description:
      "Software Engineering Team: the Leader owns goals, task breakdown, coordination, risk control, and final integration; PM, UI/UX, Frontend, Backend, Architect, QA, and Code Reviewer cover product, design, client, server, architecture, validation, and review.",
    source: "builtin",
    members: [
      baseMember({
        memberId: "leader",
        name: "engineering-lead",
        role: "leader",
        description:
          "Agents Orchestrator: owns goals, definition of done, task breakdown, dependency coordination, risk management, acceptance, and final decisions.",
        resourcePreset: "medium",
        isLeader: true,
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.agents-orchestrator",
      }),
      baseMember({
        memberId: "pm",
        name: "product-manager",
        role: "product-manager",
        description:
          "Product Manager: owns requirements, product direction, PRD, user flows, feature boundaries, priorities, and acceptance criteria.",
        agentProfileKey: "agency.product-manager",
      }),
      baseMember({
        memberId: "ui-ux",
        name: "ui-ux-designer",
        role: "ui-ux-designer",
        description:
          "UI Designer: owns visual direction, UX, interaction states, component guidance, and implementable design handoff.",
        agentProfileKey: "agency.ui-designer",
      }),
      baseMember({
        memberId: "frontend",
        name: "frontend-engineer",
        role: "frontend-engineer",
        description:
          "Frontend Developer: owns frontend UI implementation, API integration, state management, interaction behavior, responsiveness, and accessibility.",
        resourcePreset: "medium",
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.frontend-developer",
      }),
      baseMember({
        memberId: "backend",
        name: "backend-engineer",
        role: "backend-engineer",
        description:
          "Backend Architect: owns APIs, databases, permissions, queues, business logic, and server-side system capabilities.",
        resourcePreset: "medium",
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.backend-architect",
      }),
      baseMember({
        memberId: "architect",
        name: "architect",
        role: "architect",
        description:
          "Software Architect: owns technical choices, system boundaries, availability, extensibility, technical standards, and evolution plans.",
        resourcePreset: "medium",
        cpuCores: 4,
        memoryGb: 8,
        diskGb: 50,
        agentProfileKey: "agency.software-architect",
      }),
      baseMember({
        memberId: "qa",
        name: "qa-engineer",
        role: "qa-engineer",
        description:
          "Evidence Collector: owns functional validation, regression checks, evidence gathering, reproduction notes, and acceptance verdicts.",
        agentProfileKey: "agency.evidence-collector",
      }),
      baseMember({
        memberId: "code-reviewer",
        name: "code-reviewer",
        role: "code-reviewer",
        description:
          "Code Reviewer: owns code review, architecture consistency, maintainability, test coverage, risk findings, and pre-merge quality gates.",
        agentProfileKey: "agency.code-reviewer",
      }),
    ],
  },
];

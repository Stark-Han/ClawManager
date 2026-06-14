import { expect, test } from "../../fixtures/test.js";
import {
  createTeam,
  deleteTeam,
  getTeam,
  listInstances,
  login
} from "../../fixtures/apiClient.js";
import { users } from "../../fixtures/users.js";

test.describe("Team lite member mode", () => {
  test("@p0 creates Team members as Lite gateway instances", async ({ request }) => {
    const tokens = await login(request, users.admin);
    const teamName = `e2e-lite-team-${Date.now().toString(36)}`;
    let teamId: number | undefined;

    try {
      const created = await createTeam(request, tokens.access_token, {
        name: teamName,
        description: "E2E coverage for Team Lite member creation",
        communication_mode: "leader_mediated",
        shared_storage_gb: 1,
        members: [
          {
            member_id: "leader",
            name: "lite-leader",
            role: "leader",
            mode: "lite",
            instance_mode: "lite",
            runtime_type: "openclaw",
            cpu_cores: 0.1,
            memory_gb: 1,
            disk_gb: 10,
            is_leader: true
          },
          {
            member_id: "worker",
            name: "lite-worker",
            role: "developer",
            mode: "lite",
            instance_mode: "lite",
            runtime_type: "openclaw",
            cpu_cores: 0.1,
            memory_gb: 1,
            disk_gb: 10
          }
        ]
      });
      teamId = created.team.id;

      expect(created.team.name).toBe(teamName);
      expect(created.leader_member_id).toBe("leader");
      expect(created.members).toHaveLength(2);
      expect(created.members.every((member) => member.instance_mode === "lite")).toBeTruthy();
      expect(created.members.every((member) => member.runtime_type === "openclaw")).toBeTruthy();
      expect(created.members.every((member) => typeof member.instance_id === "number")).toBeTruthy();

      const details = await getTeam(request, tokens.access_token, teamId);
      expect(details.team.status).toBe("running");
      expect(details.members.map((member) => member.member_key).sort()).toEqual(["leader", "worker"]);
      expect(details.members.every((member) => member.instance_mode === "lite")).toBeTruthy();

      const instanceIds = new Set(
        details.members
          .map((member) => member.instance_id)
          .filter((id): id is number => typeof id === "number")
      );
      const instances = await listInstances(request, tokens.access_token, { admin: true, limit: 1000 });
      const memberInstances = instances.instances.filter((instance) => instanceIds.has(instance.id));
      expect(memberInstances).toHaveLength(2);
      expect(memberInstances.every((instance) => instance.instance_mode === "lite")).toBeTruthy();
      expect(memberInstances.every((instance) => instance.runtime_type === "gateway")).toBeTruthy();
    } finally {
      if (teamId !== undefined) {
        await deleteTeam(request, tokens.access_token, teamId);
      }
    }
  });
});

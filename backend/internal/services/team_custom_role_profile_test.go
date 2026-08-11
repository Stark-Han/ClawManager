package services

import (
	"strings"
	"testing"

	"clawreef/internal/models"
)

func TestPlanTeamMembersCompilesCustomRoleProfileIntoExistingIdentityFlow(t *testing.T) {
	description := "Owns evidence-backed market research."
	plans, err := planTeamMembers("research-team", []CreateTeamMemberRequest{
		{
			MemberID: "leader", Name: "Leader", Role: "leader", IsLeader: true,
			RuntimeType: "openclaw", Mode: InstanceModeLite,
			RoleProfile: &TeamMemberRoleProfileRequest{
				DisplayName: "Research Leader", RoleHint: "leader", Summary: "Coordinates the research team.",
				Mission: "Decompose the goal and synthesize the final answer.",
			},
		},
		{
			MemberID: "market-researcher", Name: "Market Researcher", Role: "researcher",
			RuntimeType: "openclaw", Mode: InstanceModeLite, Description: &description,
			RoleProfile: &TeamMemberRoleProfileRequest{
				ProfileKey: "custom.team-template.7.market-researcher", DisplayName: "Market Researcher",
				RoleHint: "market-researcher", Summary: description, Mission: "Research the market with traceable evidence.",
				Responsibilities: []string{"Compare competitors", "Cite primary sources"},
				Boundaries:       []string{"Do not implement product code"},
				Deliverables:     []string{"Competitive analysis"},
			},
		},
	})
	if err != nil {
		t.Fatalf("planTeamMembers returned error: %v", err)
	}
	if got := plans[1].EffectiveRole; got != "market-researcher" {
		t.Fatalf("EffectiveRole = %q, want market-researcher", got)
	}
	if got := plans[1].ProfileKey; got != "custom.team-template.7.market-researcher" {
		t.Fatalf("ProfileKey = %q", got)
	}
	soul := buildTeamMemberSoulMarkdown(plans[1], teamCommunicationModeLeaderMediated)
	for _, expected := range []string{"Research the market with traceable evidence.", "Compare competitors", "Do not implement product code", "Competitive analysis"} {
		if !strings.Contains(soul, expected) {
			t.Fatalf("SOUL.md missing %q:\n%s", expected, soul)
		}
	}
	if !strings.Contains(plans[1].Request.EnvironmentOverrides["CLAWMANAGER_HERMES_SYSTEM_PROMPT"], "Compare competitors") {
		t.Fatal("Hermes compatibility system prompt did not receive the custom role")
	}
	if description := plannedTeamMemberDescription(plans[1]); !strings.Contains(description, "Compare competitors") || !strings.Contains(description, "Do not implement product code") {
		t.Fatalf("Leader-facing member description did not receive the complete generated role: %s", description)
	}
	roster, err := buildTeamRosterConfig(&models.Team{
		ID: 7, CommunicationMode: teamCommunicationModeLeaderMediated, SharedMountPath: "/team",
	}, plans)
	if err != nil {
		t.Fatalf("buildTeamRosterConfig returned error: %v", err)
	}
	for _, expected := range []string{"custom.team-template.7.market-researcher", "Compare competitors", "Do not implement product code", "Competitive analysis"} {
		if !strings.Contains(roster, expected) {
			t.Fatalf("team.json roster missing %q:\n%s", expected, roster)
		}
	}
}

package systembackup

import (
	"bytes"
	"clawreef/internal/systembackup/team"
	"testing"
	"time"
)

func TestNormalizePassedPreflightRetainsOriginalWithoutResumingUpgrade(t *testing.T) {
	row := team.Row{"id": team.Value("7"), "status": team.Value("preflight_passed"), "phase": team.Value("preflight"), "preflight_json": team.Value(`{"saved":"original assessment"}`), "auto_rollback": team.Value("1")}
	table := SQLTable{Name: "runtime_rollouts", Columns: []SQLColumn{{Name: "status"}, {Name: "phase"}, {Name: "preflight_json"}, {Name: "auto_rollback"}}}
	change, e := NormalizeDataRow(table, row, time.Now().UTC())
	if e != nil {
		t.Fatal("valid preflight state rejected", e)
	}
	if string(change.Before["status"].Data) != "preflight_passed" || string(change.After["status"].Data) != "error" || string(change.After["auto_rollback"].Data) != "0" || !bytes.Equal(change.After["preflight_json"].Data, row["preflight_json"].Data) {
		t.Fatal("lost evidence or retained executable preflight")
	}
}

func TestNormalizeCurrentUpgradeIntermediateStates(t *testing.T) {
	for _, state := range []string{"prepared", "snapshotted", "restart_ready", "gateway_verified", "restored"} {
		row := team.Row{"id": team.Value("1"), "state": team.Value(state), "operation_token": team.Value("source-secret")}
		table := SQLTable{Name: "runtime_upgrade_items", Columns: []SQLColumn{{Name: "state"}, {Name: "operation_token", Nullable: "YES"}}}
		change, e := NormalizeDataRow(table, row, time.Now().UTC())
		if e != nil {
			t.Errorf("current state %s rejected: %v", state, e)
			continue
		}
		if string(change.Before["state"].Data) != state || string(change.After["state"].Data) != "failed" || !change.After["operation_token"].IsNull || !change.Quarantined {
			t.Errorf("unsafe intermediate upgrade restore: %+v", change)
		}
	}
}

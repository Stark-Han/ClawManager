package systembackup

import (
	"clawreef/internal/systembackup/team"
	"testing"
	"time"
)

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

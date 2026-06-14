package models

import "time"

type InstanceRuntimeBinding struct {
	ID            int64      `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID    int        `db:"instance_id" json:"instance_id"`
	RuntimePodID  int64      `db:"runtime_pod_id" json:"runtime_pod_id"`
	RuntimeType   string     `db:"runtime_type" json:"runtime_type"`
	GatewayID     string     `db:"gateway_id" json:"gateway_id"`
	GatewayPort   int        `db:"gateway_port" json:"gateway_port"`
	GatewayPID    *int       `db:"gateway_pid" json:"gateway_pid,omitempty"`
	WorkspacePath string     `db:"workspace_path" json:"workspace_path"`
	State         string     `db:"state" json:"state"`
	Generation    int        `db:"generation" json:"generation"`
	LastHealthAt  *time.Time `db:"last_health_at" json:"last_health_at,omitempty"`
	ErrorMessage  *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

func (InstanceRuntimeBinding) TableName() string {
	return "instance_runtime_bindings"
}

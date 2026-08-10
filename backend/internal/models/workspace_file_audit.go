package models

import "time"

type WorkspaceFileAudit struct {
	ID           int64     `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID   int       `db:"instance_id" json:"instance_id"`
	UserID       int       `db:"user_id" json:"user_id"`
	Action       string    `db:"action" json:"action"`
	RelativePath string    `db:"relative_path" json:"relative_path"`
	Bytes        int64     `db:"bytes" json:"bytes"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

func (WorkspaceFileAudit) TableName() string {
	return "workspace_file_audits"
}

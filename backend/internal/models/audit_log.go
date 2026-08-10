package models

import (
	"time"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           int       `db:"id,primarykey,autoincrement" json:"id"`
	UserID       *int      `db:"user_id" json:"user_id,omitempty"`
	Action       string    `db:"action" json:"action"`
	ResourceType string    `db:"resource_type" json:"resource_type"`
	ResourceID   *int      `db:"resource_id" json:"resource_id,omitempty"`
	Details      *string   `db:"details" json:"details,omitempty"`
	IPAddress    *string   `db:"ip_address" json:"ip_address,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// TableName returns the table name for the AuditLog model
func (a AuditLog) TableName() string {
	return "audit_logs"
}

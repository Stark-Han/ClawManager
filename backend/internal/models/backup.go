package models

import (
	"time"
)

// Backup represents a backup of an instance
type Backup struct {
	ID           int        `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID   int        `db:"instance_id" json:"instance_id"`
	BackupName   string     `db:"backup_name" json:"backup_name"`
	BackupSizeGB *int       `db:"backup_size_gb" json:"backup_size_gb,omitempty"`
	BackupPath   *string    `db:"backup_path" json:"backup_path,omitempty"`
	Status       string     `db:"status" json:"status"`
	BackupType   string     `db:"backup_type" json:"backup_type"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	ExpiresAt    *time.Time `db:"expires_at" json:"expires_at,omitempty"`
}

// TableName returns the table name for the Backup model
func (b Backup) TableName() string {
	return "backups"
}

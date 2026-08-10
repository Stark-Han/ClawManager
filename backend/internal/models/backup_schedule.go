package models

import (
	"time"
)

// BackupSchedule represents a scheduled backup task
type BackupSchedule struct {
	ID             int       `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID     int       `db:"instance_id" json:"instance_id"`
	ScheduleName   *string   `db:"schedule_name" json:"schedule_name,omitempty"`
	CronExpression string    `db:"cron_expression" json:"cron_expression"`
	RetentionDays  int       `db:"retention_days" json:"retention_days"`
	IsActive       bool      `db:"is_active" json:"is_active"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// TableName returns the table name for the BackupSchedule model
func (b BackupSchedule) TableName() string {
	return "backup_schedules"
}

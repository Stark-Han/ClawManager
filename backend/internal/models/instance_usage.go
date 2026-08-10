package models

import (
	"time"
)

// InstanceUsage represents resource usage statistics for an instance
type InstanceUsage struct {
	ID              int       `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID      int       `db:"instance_id" json:"instance_id"`
	CPUUsagePercent *float64  `db:"cpu_usage_percent" json:"cpu_usage_percent,omitempty"`
	MemoryUsageGB   *float64  `db:"memory_usage_gb" json:"memory_usage_gb,omitempty"`
	DiskUsageGB     *float64  `db:"disk_usage_gb" json:"disk_usage_gb,omitempty"`
	GPUUsagePercent *float64  `db:"gpu_usage_percent" json:"gpu_usage_percent,omitempty"`
	UptimeSeconds   *int      `db:"uptime_seconds" json:"uptime_seconds,omitempty"`
	RecordedAt      time.Time `db:"recorded_at" json:"recorded_at"`
}

// TableName returns the table name for the InstanceUsage model
func (i InstanceUsage) TableName() string {
	return "instance_usage"
}

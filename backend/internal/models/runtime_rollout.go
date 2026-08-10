package models

import "time"

type RuntimeRollout struct {
	ID             int64      `db:"id,primarykey,autoincrement" json:"id"`
	RuntimeType    string     `db:"runtime_type" json:"runtime_type"`
	TargetImageRef string     `db:"target_image_ref" json:"target_image_ref"`
	Status         string     `db:"status" json:"status"`
	BatchSize      int        `db:"batch_size" json:"batch_size"`
	MaxUnavailable int        `db:"max_unavailable" json:"max_unavailable"`
	StartedBy      *int       `db:"started_by" json:"started_by,omitempty"`
	StartedAt      *time.Time `db:"started_at" json:"started_at,omitempty"`
	FinishedAt     *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	ErrorMessage   *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
}

func (RuntimeRollout) TableName() string {
	return "runtime_rollouts"
}

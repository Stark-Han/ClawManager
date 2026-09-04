package models

import "time"

type OpenClawUpgradeLabRun struct {
	ID                  int64      `db:"id,primarykey,autoincrement" json:"id"`
	ActorUserID         *int       `db:"actor_user_id" json:"actor_user_id,omitempty"`
	Status              string     `db:"status" json:"status"`
	Phase               string     `db:"phase" json:"phase"`
	BaselineImageRef    string     `db:"baseline_image_ref" json:"baseline_image_ref"`
	BaselineImageDigest string     `db:"baseline_image_digest" json:"baseline_image_digest"`
	SourceDeployment    string     `db:"source_deployment" json:"source_deployment"`
	InstanceIDsJSON     *string    `db:"instance_ids_json" json:"-"`
	TargetImageRef      *string    `db:"target_image_ref" json:"target_image_ref,omitempty"`
	RolloutID           *int64     `db:"rollout_id" json:"rollout_id,omitempty"`
	BeforeJSON          *string    `db:"before_json" json:"-"`
	ChecksJSON          *string    `db:"checks_json" json:"-"`
	ErrorCode           *string    `db:"error_code" json:"error_code,omitempty"`
	ErrorMessage        *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
	FinishedAt          *time.Time `db:"finished_at" json:"finished_at,omitempty"`
}

func (OpenClawUpgradeLabRun) TableName() string { return "openclaw_upgrade_lab_runs" }

package models

import "time"

type RuntimeRollout struct {
	ID                       int64      `db:"id,primarykey,autoincrement" json:"id"`
	RuntimeType              string     `db:"runtime_type" json:"runtime_type"`
	TargetImageRef           string     `db:"target_image_ref" json:"target_image_ref"`
	SourceImagesJSON         *string    `db:"source_images_json" json:"-"`
	TargetImageDigest        *string    `db:"target_image_digest" json:"target_image_digest,omitempty"`
	Status                   string     `db:"status" json:"status"`
	Phase                    string     `db:"phase" json:"phase"`
	PreflightID              *string    `db:"preflight_id" json:"preflight_id,omitempty"`
	PlanFingerprint          *string    `db:"plan_fingerprint" json:"plan_fingerprint,omitempty"`
	PreflightJSON            *string    `db:"preflight_json" json:"-"`
	RequiredCapabilitiesJSON *string    `db:"required_capabilities_json" json:"-"`
	AutoRollback             bool       `db:"auto_rollback" json:"auto_rollback"`
	RollbackStatus           *string    `db:"rollback_status" json:"rollback_status,omitempty"`
	RollbackError            *string    `db:"rollback_error" json:"rollback_error,omitempty"`
	BatchSize                int        `db:"batch_size" json:"batch_size"`
	MaxUnavailable           int        `db:"max_unavailable" json:"max_unavailable"`
	StartedBy                *int       `db:"started_by" json:"started_by,omitempty"`
	StartedAt                *time.Time `db:"started_at" json:"started_at,omitempty"`
	FinishedAt               *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	ErrorMessage             *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt                time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt                time.Time  `db:"updated_at" json:"updated_at"`
}

func (RuntimeRollout) TableName() string {
	return "runtime_rollouts"
}

type RuntimeUpgradeItem struct {
	ID                      int64      `db:"id,primarykey,autoincrement" json:"id"`
	RolloutID               int64      `db:"rollout_id" json:"rollout_id"`
	TeamID                  *int       `db:"team_id" json:"team_id,omitempty"`
	TeamMemberID            *int       `db:"team_member_id" json:"team_member_id,omitempty"`
	InstanceID              int        `db:"instance_id" json:"instance_id"`
	RuntimePodID            *int64     `db:"runtime_pod_id" json:"runtime_pod_id,omitempty"`
	MemberOrder             int        `db:"member_order" json:"member_order"`
	IsTeamLeader            bool       `db:"is_team_leader" json:"is_team_leader"`
	SourceRuntimeVersion    *string    `db:"source_runtime_version" json:"source_runtime_version,omitempty"`
	TargetRuntimeVersion    *string    `db:"target_runtime_version" json:"target_runtime_version,omitempty"`
	State                   string     `db:"state" json:"state"`
	WorkspaceManifestSHA256 *string    `db:"workspace_manifest_sha256" json:"workspace_manifest_sha256,omitempty"`
	SnapshotRef             *string    `db:"snapshot_ref" json:"-"`
	PreflightJSON           *string    `db:"preflight_json" json:"-"`
	PostflightJSON          *string    `db:"postflight_json" json:"-"`
	ErrorMessage            *string    `db:"error_message" json:"error_message,omitempty"`
	StartedAt               *time.Time `db:"started_at" json:"started_at,omitempty"`
	FinishedAt              *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	CreatedAt               time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time  `db:"updated_at" json:"updated_at"`
}

func (RuntimeUpgradeItem) TableName() string { return "runtime_upgrade_items" }

type RuntimeUpgradeAudit struct {
	ID          int64     `db:"id,primarykey,autoincrement" json:"id"`
	RolloutID   *int64    `db:"rollout_id" json:"rollout_id,omitempty"`
	ActorUserID *int      `db:"actor_user_id" json:"actor_user_id,omitempty"`
	Action      string    `db:"action" json:"action"`
	Phase       string    `db:"phase" json:"phase"`
	Outcome     string    `db:"outcome" json:"outcome"`
	DetailJSON  *string   `db:"detail_json" json:"-"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

func (RuntimeUpgradeAudit) TableName() string { return "runtime_upgrade_audits" }

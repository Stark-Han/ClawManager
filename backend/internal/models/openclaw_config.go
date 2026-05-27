package models

import "time"

// OpenClawConfigResource stores a reusable OpenClaw bootstrap resource.
type OpenClawConfigResource struct {
	ID           int       `db:"id,primarykey,autoincrement" json:"id"`
	UserID       int       `db:"user_id" json:"user_id"`
	ResourceType string    `db:"resource_type" json:"resource_type"`
	ResourceKey  string    `db:"resource_key" json:"resource_key"`
	Name         string    `db:"name" json:"name"`
	Description  *string   `db:"description" json:"description,omitempty"`
	Enabled      bool      `db:"enabled" json:"enabled"`
	Version      int       `db:"version" json:"version"`
	TagsJSON     string    `db:"tags_json" json:"-"`
	ContentJSON  string    `db:"content_json" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

func (r OpenClawConfigResource) TableName() string {
	return "openclaw_config_resources"
}

// OpenClawConfigBundle stores a reusable bundle of OpenClaw resources.
type OpenClawConfigBundle struct {
	ID          int       `db:"id,primarykey,autoincrement" json:"id"`
	UserID      int       `db:"user_id" json:"user_id"`
	Name        string    `db:"name" json:"name"`
	Description *string   `db:"description" json:"description,omitempty"`
	Enabled     bool      `db:"enabled" json:"enabled"`
	Version     int       `db:"version" json:"version"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

func (b OpenClawConfigBundle) TableName() string {
	return "openclaw_config_bundles"
}

// OpenClawConfigBundleItem stores a resource membership inside a bundle.
type OpenClawConfigBundleItem struct {
	ID         int  `db:"id,primarykey,autoincrement" json:"id"`
	BundleID   int  `db:"bundle_id" json:"bundle_id"`
	ResourceID int  `db:"resource_id" json:"resource_id"`
	SortOrder  int  `db:"sort_order" json:"sort_order"`
	Required   bool `db:"required" json:"required"`
}

func (i OpenClawConfigBundleItem) TableName() string {
	return "openclaw_config_bundle_items"
}

// OpenClawConfigBundleSkill stores an uploaded skill membership inside a bundle.
type OpenClawConfigBundleSkill struct {
	ID        int       `db:"id,primarykey,autoincrement" json:"id"`
	BundleID  int       `db:"bundle_id" json:"bundle_id"`
	SkillID   int       `db:"skill_id" json:"skill_id"`
	SortOrder int       `db:"sort_order" json:"sort_order"`
	Required  bool      `db:"required" json:"required"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (i OpenClawConfigBundleSkill) TableName() string {
	return "openclaw_config_bundle_skills"
}

// OpenClawInjectionSnapshot stores the rendered bootstrap payload used by an instance.
type OpenClawInjectionSnapshot struct {
	ID                      int        `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID              *int       `db:"instance_id" json:"instance_id,omitempty"`
	UserID                  int        `db:"user_id" json:"user_id"`
	Mode                    string     `db:"mode" json:"mode"`
	BundleID                *int       `db:"bundle_id" json:"bundle_id,omitempty"`
	SelectedResourceIDsJSON string     `db:"selected_resource_ids_json" json:"-"`
	ResolvedResourcesJSON   string     `db:"resolved_resources_json" json:"-"`
	RenderedManifestJSON    string     `db:"rendered_manifest_json" json:"-"`
	RenderedEnvJSON         string     `db:"rendered_env_json" json:"-"`
	SecretName              *string    `db:"secret_name" json:"secret_name,omitempty"`
	Status                  string     `db:"status" json:"status"`
	ErrorMessage            *string    `db:"error_message" json:"error_message,omitempty"`
	CreatedAt               time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time  `db:"updated_at" json:"updated_at"`
	ActivatedAt             *time.Time `db:"activated_at" json:"activated_at,omitempty"`
}

func (s OpenClawInjectionSnapshot) TableName() string {
	return "openclaw_injection_snapshots"
}

package models

import (
	"time"
)

// PersistentVolume represents a persistent storage volume
type PersistentVolume struct {
	ID            int       `db:"id,primarykey,autoincrement" json:"id"`
	InstanceID    int       `db:"instance_id" json:"instance_id"`
	PVCName       string    `db:"pvc_name" json:"pvc_name"`
	PVCNamespace  string    `db:"pvc_namespace" json:"pvc_namespace"`
	StorageSizeGB int       `db:"storage_size_gb" json:"storage_size_gb"`
	StorageClass  *string   `db:"storage_class" json:"storage_class,omitempty"`
	MountPath     *string   `db:"mount_path" json:"mount_path,omitempty"`
	Status        string    `db:"status" json:"status"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// TableName returns the table name for the PersistentVolume model
func (p PersistentVolume) TableName() string {
	return "persistent_volumes"
}

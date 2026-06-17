package repository

import (
	"context"
	"fmt"
	"time"

	"clawreef/internal/models"

	"github.com/upper/db/v4"
)

type WorkspaceFileAuditRepository interface {
	Create(ctx context.Context, audit *models.WorkspaceFileAudit) error
}

type workspaceFileAuditRepository struct {
	sess db.Session
}

func NewWorkspaceFileAuditRepository(sess db.Session) WorkspaceFileAuditRepository {
	return &workspaceFileAuditRepository{sess: sess}
}

func (r *workspaceFileAuditRepository) Create(ctx context.Context, audit *models.WorkspaceFileAudit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = time.Now().UTC()
	}
	res, err := r.sess.Collection("workspace_file_audits").Insert(audit)
	if err != nil {
		return fmt.Errorf("failed to create workspace file audit: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		audit.ID = id
	}
	return nil
}

package repository

import (
	"fmt"
	"time"

	"clawreef/internal/models"

	"github.com/upper/db/v4"
)

type SecurityScanRepository interface {
	GetConfig() (*models.SecurityScanConfig, error)
	UpsertConfig(config *models.SecurityScanConfig) error
	CreateJob(job *models.SecurityScanJob) error
	UpdateJob(job *models.SecurityScanJob) error
	GetJobByID(id int) (*models.SecurityScanJob, error)
	ListJobs(limit int) ([]models.SecurityScanJob, error)
	CreateJobItem(item *models.SecurityScanJobItem) error
	UpdateJobItem(item *models.SecurityScanJobItem) error
	ListJobItems(jobID int) ([]models.SecurityScanJobItem, error)
	UpsertReport(report *models.SecurityScanReport) error
	GetReportByJobID(jobID int) (*models.SecurityScanReport, error)
}

type securityScanRepository struct {
	sess db.Session
}

func NewSecurityScanRepository(sess db.Session) SecurityScanRepository {
	repo := &securityScanRepository{sess: sess}
	repo.ensureTables()
	return repo
}

func (r *securityScanRepository) ensureTables() {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS security_scan_configs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			default_mode VARCHAR(20) NOT NULL DEFAULT 'quick',
			quick_analyzers_json LONGTEXT NOT NULL,
			deep_analyzers_json LONGTEXT NOT NULL,
			quick_timeout_seconds INT NOT NULL DEFAULT 30,
			deep_timeout_seconds INT NOT NULL DEFAULT 120,
			allow_fallback BOOLEAN NOT NULL DEFAULT TRUE,
			updated_by INT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_security_scan_configs_singleton (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS security_scan_jobs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			asset_type VARCHAR(30) NOT NULL DEFAULT 'skill',
			scan_mode VARCHAR(20) NOT NULL DEFAULT 'quick',
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			requested_by INT NULL,
			scope_json LONGTEXT NULL,
			total_items INT NOT NULL DEFAULT 0,
			completed_items INT NOT NULL DEFAULT 0,
			failed_items INT NOT NULL DEFAULT 0,
			current_item_name VARCHAR(255) NULL,
			started_at TIMESTAMP NULL,
			finished_at TIMESTAMP NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_security_scan_jobs_status (status, created_at),
			INDEX idx_security_scan_jobs_asset_type (asset_type, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS security_scan_job_items (
			id INT AUTO_INCREMENT PRIMARY KEY,
			job_id INT NOT NULL,
			asset_type VARCHAR(30) NOT NULL DEFAULT 'skill',
			asset_id INT NOT NULL,
			asset_name VARCHAR(255) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			progress_pct INT NOT NULL DEFAULT 0,
			risk_level VARCHAR(30) NULL,
			summary TEXT NULL,
			scan_result_id INT NULL,
			cached_result BOOLEAN NOT NULL DEFAULT FALSE,
			error_message TEXT NULL,
			started_at TIMESTAMP NULL,
			finished_at TIMESTAMP NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			FOREIGN KEY (job_id) REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
			UNIQUE KEY uk_security_scan_job_items_job_asset (job_id, asset_type, asset_id),
			INDEX idx_security_scan_job_items_job (job_id, status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS security_scan_reports (
			id INT AUTO_INCREMENT PRIMARY KEY,
			job_id INT NOT NULL,
			summary_json LONGTEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			FOREIGN KEY (job_id) REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
			UNIQUE KEY uk_security_scan_reports_job (job_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, statement := range statements {
		if _, err := r.sess.SQL().Exec(statement); err != nil {
			panic(fmt.Errorf("failed to ensure security scan tables: %w", err))
		}
	}
}

func (r *securityScanRepository) GetConfig() (*models.SecurityScanConfig, error) {
	var item models.SecurityScanConfig
	err := r.sess.Collection("security_scan_configs").Find(db.Cond{"id": 1}).One(&item)
	if err == db.ErrNoMoreRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security scan config: %w", err)
	}
	return &item, nil
}

func (r *securityScanRepository) UpsertConfig(config *models.SecurityScanConfig) error {
	existing, err := r.GetConfig()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if existing == nil {
		config.ID = 1
		config.CreatedAt = now
		config.UpdatedAt = now
		if _, err := r.sess.Collection("security_scan_configs").Insert(config); err != nil {
			return fmt.Errorf("failed to create security scan config: %w", err)
		}
		return nil
	}
	config.ID = existing.ID
	config.CreatedAt = existing.CreatedAt
	config.UpdatedAt = now
	if err := r.sess.Collection("security_scan_configs").Find(db.Cond{"id": existing.ID}).Update(config); err != nil {
		return fmt.Errorf("failed to update security scan config: %w", err)
	}
	return nil
}

func (r *securityScanRepository) CreateJob(job *models.SecurityScanJob) error {
	ensureTimestamps(&job.CreatedAt, &job.UpdatedAt)
	res, err := r.sess.Collection("security_scan_jobs").Insert(job)
	if err != nil {
		return fmt.Errorf("failed to create security scan job: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		job.ID = int(id)
	}
	return nil
}

func (r *securityScanRepository) UpdateJob(job *models.SecurityScanJob) error {
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = time.Now().UTC()
	}
	if err := r.sess.Collection("security_scan_jobs").Find(db.Cond{"id": job.ID}).Update(job); err != nil {
		return fmt.Errorf("failed to update security scan job: %w", err)
	}
	return nil
}

func (r *securityScanRepository) GetJobByID(id int) (*models.SecurityScanJob, error) {
	var item models.SecurityScanJob
	err := r.sess.Collection("security_scan_jobs").Find(db.Cond{"id": id}).One(&item)
	if err == db.ErrNoMoreRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security scan job: %w", err)
	}
	return &item, nil
}

func (r *securityScanRepository) ListJobs(limit int) ([]models.SecurityScanJob, error) {
	if limit <= 0 {
		limit = 20
	}
	var items []models.SecurityScanJob
	if err := r.sess.Collection("security_scan_jobs").Find().OrderBy("-created_at", "-id").Limit(limit).All(&items); err != nil {
		return nil, fmt.Errorf("failed to list security scan jobs: %w", err)
	}
	return items, nil
}

func (r *securityScanRepository) CreateJobItem(item *models.SecurityScanJobItem) error {
	ensureTimestamps(&item.CreatedAt, &item.UpdatedAt)
	res, err := r.sess.Collection("security_scan_job_items").Insert(item)
	if err != nil {
		return fmt.Errorf("failed to create security scan job item: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		item.ID = int(id)
	}
	return nil
}

func (r *securityScanRepository) UpdateJobItem(item *models.SecurityScanJobItem) error {
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	if err := r.sess.Collection("security_scan_job_items").Find(db.Cond{"id": item.ID}).Update(item); err != nil {
		return fmt.Errorf("failed to update security scan job item: %w", err)
	}
	return nil
}

func (r *securityScanRepository) ListJobItems(jobID int) ([]models.SecurityScanJobItem, error) {
	var items []models.SecurityScanJobItem
	if err := r.sess.Collection("security_scan_job_items").Find(db.Cond{"job_id": jobID}).OrderBy("id").All(&items); err != nil {
		return nil, fmt.Errorf("failed to list security scan job items: %w", err)
	}
	return items, nil
}

func (r *securityScanRepository) UpsertReport(report *models.SecurityScanReport) error {
	var existing models.SecurityScanReport
	err := r.sess.Collection("security_scan_reports").Find(db.Cond{"job_id": report.JobID}).One(&existing)
	if err == db.ErrNoMoreRows {
		ensureTimestamps(&report.CreatedAt, &report.UpdatedAt)
		res, err := r.sess.Collection("security_scan_reports").Insert(report)
		if err != nil {
			return fmt.Errorf("failed to create security scan report: %w", err)
		}
		if id, ok := res.ID().(int64); ok {
			report.ID = int(id)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get security scan report: %w", err)
	}
	report.ID = existing.ID
	report.CreatedAt = existing.CreatedAt
	report.UpdatedAt = time.Now().UTC()
	if err := r.sess.Collection("security_scan_reports").Find(db.Cond{"id": existing.ID}).Update(report); err != nil {
		return fmt.Errorf("failed to update security scan report: %w", err)
	}
	return nil
}

func (r *securityScanRepository) GetReportByJobID(jobID int) (*models.SecurityScanReport, error) {
	var item models.SecurityScanReport
	err := r.sess.Collection("security_scan_reports").Find(db.Cond{"job_id": jobID}).One(&item)
	if err == db.ErrNoMoreRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security scan report: %w", err)
	}
	return &item, nil
}

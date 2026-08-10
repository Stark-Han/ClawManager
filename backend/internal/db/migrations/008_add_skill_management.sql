CREATE TABLE IF NOT EXISTS skill_blobs (
  id INT AUTO_INCREMENT PRIMARY KEY,
  content_hash VARCHAR(128) NOT NULL,
  archive_hash VARCHAR(128) NOT NULL,
  object_key VARCHAR(512) NOT NULL,
  file_name VARCHAR(255) NOT NULL,
  media_type VARCHAR(100) NOT NULL DEFAULT 'application/gzip',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  scan_status VARCHAR(30) NOT NULL DEFAULT 'pending',
  risk_level VARCHAR(30) NOT NULL DEFAULT 'unknown',
  last_scanned_at TIMESTAMP NULL,
  last_scan_result_id INT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_skill_blobs_content_hash (content_hash),
  INDEX idx_skill_blobs_scan_status (scan_status),
  INDEX idx_skill_blobs_risk_level (risk_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS skills (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  skill_key VARCHAR(120) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT NULL,
  current_version_id INT NULL,
  source_type VARCHAR(30) NOT NULL DEFAULT 'uploaded',
  status VARCHAR(30) NOT NULL DEFAULT 'active',
  risk_level VARCHAR(30) NOT NULL DEFAULT 'unknown',
  last_scanned_at TIMESTAMP NULL,
  last_scan_result_id INT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  UNIQUE KEY uk_skills_user_key (user_id, skill_key),
  INDEX idx_skills_user_status (user_id, status),
  INDEX idx_skills_risk_level (risk_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS skill_versions (
  id INT AUTO_INCREMENT PRIMARY KEY,
  skill_id INT NOT NULL,
  blob_id INT NOT NULL,
  version_no INT NOT NULL,
  manifest_json LONGTEXT NULL,
  source_type VARCHAR(30) NOT NULL DEFAULT 'uploaded',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE,
  FOREIGN KEY (blob_id) REFERENCES skill_blobs(id) ON DELETE RESTRICT,
  UNIQUE KEY uk_skill_versions_skill_version (skill_id, version_no),
  UNIQUE KEY uk_skill_versions_skill_blob (skill_id, blob_id),
  INDEX idx_skill_versions_skill_id (skill_id, version_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS instance_skills (
  id INT AUTO_INCREMENT PRIMARY KEY,
  instance_id INT NOT NULL,
  skill_id INT NOT NULL,
  skill_version_id INT NULL,
  source_type VARCHAR(40) NOT NULL DEFAULT 'discovered_in_instance',
  install_path VARCHAR(1024) NULL,
  observed_hash VARCHAR(128) NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'active',
  last_seen_at TIMESTAMP NULL,
  removed_at TIMESTAMP NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE,
  FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE,
  FOREIGN KEY (skill_version_id) REFERENCES skill_versions(id) ON DELETE SET NULL,
  UNIQUE KEY uk_instance_skills_instance_skill (instance_id, skill_id),
  INDEX idx_instance_skills_instance (instance_id, status),
  INDEX idx_instance_skills_skill (skill_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS skill_scan_results (
  id INT AUTO_INCREMENT PRIMARY KEY,
  blob_id INT NOT NULL,
  engine VARCHAR(60) NOT NULL,
  risk_level VARCHAR(30) NOT NULL DEFAULT 'unknown',
  status VARCHAR(30) NOT NULL DEFAULT 'completed',
  summary TEXT NULL,
  findings_json LONGTEXT NULL,
  scanned_at TIMESTAMP NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (blob_id) REFERENCES skill_blobs(id) ON DELETE CASCADE,
  INDEX idx_skill_scan_results_blob (blob_id, scanned_at),
  INDEX idx_skill_scan_results_risk (risk_level, scanned_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

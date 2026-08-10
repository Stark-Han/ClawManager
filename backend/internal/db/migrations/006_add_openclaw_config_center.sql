SET @openclaw_snapshot_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instances'
    AND COLUMN_NAME = 'openclaw_config_snapshot_id'
);
SET @openclaw_snapshot_column_sql = IF(
  @openclaw_snapshot_column_exists = 0,
  'ALTER TABLE instances ADD COLUMN openclaw_config_snapshot_id INT NULL AFTER access_token',
  'SELECT 1'
);
PREPARE openclaw_snapshot_column_stmt FROM @openclaw_snapshot_column_sql;
EXECUTE openclaw_snapshot_column_stmt;
DEALLOCATE PREPARE openclaw_snapshot_column_stmt;

CREATE TABLE IF NOT EXISTS openclaw_config_resources (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  resource_type VARCHAR(50) NOT NULL,
  resource_key VARCHAR(100) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INT NOT NULL DEFAULT 1,
  tags_json LONGTEXT NOT NULL,
  content_json LONGTEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  UNIQUE KEY uk_openclaw_resource_key (user_id, resource_type, resource_key),
  INDEX idx_openclaw_resource_user_type (user_id, resource_type),
  INDEX idx_openclaw_resource_user_enabled (user_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS openclaw_config_bundles (
  id INT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  INDEX idx_openclaw_bundle_user (user_id),
  INDEX idx_openclaw_bundle_user_enabled (user_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS openclaw_config_bundle_items (
  id INT AUTO_INCREMENT PRIMARY KEY,
  bundle_id INT NOT NULL,
  resource_id INT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  required BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (bundle_id) REFERENCES openclaw_config_bundles(id) ON DELETE CASCADE,
  FOREIGN KEY (resource_id) REFERENCES openclaw_config_resources(id) ON DELETE CASCADE,
  UNIQUE KEY uk_openclaw_bundle_resource (bundle_id, resource_id),
  INDEX idx_openclaw_bundle_item_bundle (bundle_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS openclaw_injection_snapshots (
  id INT AUTO_INCREMENT PRIMARY KEY,
  instance_id INT NULL,
  user_id INT NOT NULL,
  mode VARCHAR(20) NOT NULL,
  bundle_id INT NULL,
  selected_resource_ids_json LONGTEXT NOT NULL,
  resolved_resources_json LONGTEXT NOT NULL,
  rendered_manifest_json LONGTEXT NOT NULL,
  rendered_env_json LONGTEXT NOT NULL,
  secret_name VARCHAR(255) NULL,
  status VARCHAR(30) NOT NULL DEFAULT 'pending',
  error_message TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  activated_at TIMESTAMP NULL,
  INDEX idx_openclaw_snapshot_user_created (user_id, created_at),
  INDEX idx_openclaw_snapshot_instance (instance_id),
  INDEX idx_openclaw_snapshot_bundle (bundle_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

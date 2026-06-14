SET @stmt = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'instances' AND COLUMN_NAME = 'workspace_path') = 0,
  'ALTER TABLE instances ADD COLUMN workspace_path VARCHAR(1024) NULL AFTER mount_path',
  'SELECT 1'
);
PREPARE stmt FROM @stmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @stmt = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'instances' AND COLUMN_NAME = 'workspace_usage_bytes') = 0,
  'ALTER TABLE instances ADD COLUMN workspace_usage_bytes BIGINT NOT NULL DEFAULT 0 AFTER workspace_path',
  'SELECT 1'
);
PREPARE stmt FROM @stmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @stmt = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'instances' AND COLUMN_NAME = 'runtime_generation') = 0,
  'ALTER TABLE instances ADD COLUMN runtime_generation INT NOT NULL DEFAULT 1 AFTER workspace_usage_bytes',
  'SELECT 1'
);
PREPARE stmt FROM @stmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @stmt = IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'instances' AND COLUMN_NAME = 'runtime_error_message') = 0,
  'ALTER TABLE instances ADD COLUMN runtime_error_message TEXT NULL AFTER runtime_generation',
  'SELECT 1'
);
PREPARE stmt FROM @stmt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE instances
  MODIFY COLUMN runtime_type ENUM('desktop', 'shell', 'gateway') NOT NULL DEFAULT 'desktop';

CREATE TABLE IF NOT EXISTS runtime_pods (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  runtime_type VARCHAR(32) NOT NULL,
  namespace VARCHAR(128) NOT NULL,
  pod_name VARCHAR(255) NOT NULL,
  pod_uid VARCHAR(128) NULL,
  pod_ip VARCHAR(64) NULL,
  node_name VARCHAR(255) NULL,
  deployment_name VARCHAR(255) NOT NULL,
  image_ref VARCHAR(512) NOT NULL,
  agent_endpoint VARCHAR(255) NULL,
  state VARCHAR(32) NOT NULL DEFAULT 'pending',
  capacity INT NOT NULL DEFAULT 100,
  used_slots INT NOT NULL DEFAULT 0,
  draining TINYINT(1) NOT NULL DEFAULT 0,
  cpu_millis_used BIGINT NOT NULL DEFAULT 0,
  memory_bytes_used BIGINT NOT NULL DEFAULT 0,
  disk_bytes_used BIGINT NOT NULL DEFAULT 0,
  network_rx_bytes BIGINT NOT NULL DEFAULT 0,
  network_tx_bytes BIGINT NOT NULL DEFAULT 0,
  metrics_json JSON NULL,
  last_seen_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_runtime_pod_namespace_name (namespace, pod_name),
  KEY idx_runtime_pods_schedulable (runtime_type, state, draining, used_slots, capacity),
  KEY idx_runtime_pods_last_seen (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS instance_runtime_bindings (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  instance_id INT NOT NULL,
  runtime_pod_id BIGINT NOT NULL,
  runtime_type VARCHAR(32) NOT NULL,
  gateway_id VARCHAR(128) NOT NULL,
  gateway_port INT NOT NULL,
  gateway_pid INT NULL,
  workspace_path VARCHAR(1024) NOT NULL,
  state VARCHAR(32) NOT NULL DEFAULT 'creating',
  generation INT NOT NULL DEFAULT 1,
  last_health_at DATETIME NULL,
  error_message TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_instance_runtime_binding_instance (instance_id),
  UNIQUE KEY uk_instance_runtime_binding_gateway (runtime_pod_id, gateway_port),
  KEY idx_instance_runtime_binding_pod_state (runtime_pod_id, state),
  CONSTRAINT fk_instance_runtime_binding_instance
    FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE,
  CONSTRAINT fk_instance_runtime_binding_pod
    FOREIGN KEY (runtime_pod_id) REFERENCES runtime_pods(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS runtime_rollouts (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  runtime_type VARCHAR(32) NOT NULL,
  target_image_ref VARCHAR(512) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  batch_size INT NOT NULL DEFAULT 1,
  max_unavailable INT NOT NULL DEFAULT 1,
  started_by INT NULL,
  started_at DATETIME NULL,
  finished_at DATETIME NULL,
  error_message TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_runtime_rollouts_type_status (runtime_type, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS workspace_file_audits (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  instance_id INT NOT NULL,
  user_id INT NOT NULL,
  action VARCHAR(32) NOT NULL,
  relative_path VARCHAR(1024) NOT NULL,
  bytes BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_workspace_file_audits_instance_time (instance_id, created_at),
  KEY idx_workspace_file_audits_user_time (user_id, created_at),
  CONSTRAINT fk_workspace_file_audits_instance
    FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

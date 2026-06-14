CREATE TABLE IF NOT EXISTS model_invocations (
  id INT AUTO_INCREMENT PRIMARY KEY,
  trace_id VARCHAR(100) NOT NULL,
  session_id VARCHAR(100) NULL,
  request_id VARCHAR(100) NOT NULL,
  user_id INT NULL,
  instance_id INT NULL,
  instance_mode VARCHAR(16) NULL,
  runtime_type VARCHAR(32) NULL,
  gateway_id VARCHAR(128) NULL,
  runtime_pod_id BIGINT NULL,
  model_id INT NULL,
  provider_type VARCHAR(100) NOT NULL,
  requested_model VARCHAR(255) NOT NULL,
  actual_provider_model VARCHAR(255) NOT NULL,
  traffic_class VARCHAR(50) NOT NULL,
  request_payload LONGTEXT NULL,
  response_payload LONGTEXT NULL,
  prompt_tokens INT NOT NULL DEFAULT 0,
  completion_tokens INT NOT NULL DEFAULT 0,
  total_tokens INT NOT NULL DEFAULT 0,
  cached_tokens INT NULL,
  reasoning_tokens INT NULL,
  latency_ms INT NULL,
  is_streaming BOOLEAN NOT NULL DEFAULT FALSE,
  status VARCHAR(50) NOT NULL,
  error_message TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMP NULL,
  INDEX idx_model_invocations_trace_id (trace_id),
  INDEX idx_model_invocations_session_id (session_id),
  INDEX idx_model_invocations_request_id (request_id),
  INDEX idx_model_invocations_user_id (user_id),
  INDEX idx_model_invocations_instance_id (instance_id),
  INDEX idx_model_invocations_gateway_id (gateway_id),
  INDEX idx_model_invocations_model_id (model_id),
  INDEX idx_model_invocations_status (status),
  INDEX idx_model_invocations_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_events (
  id INT AUTO_INCREMENT PRIMARY KEY,
  trace_id VARCHAR(100) NOT NULL,
  session_id VARCHAR(100) NULL,
  request_id VARCHAR(100) NULL,
  user_id INT NULL,
  instance_id INT NULL,
  instance_mode VARCHAR(16) NULL,
  runtime_type VARCHAR(32) NULL,
  gateway_id VARCHAR(128) NULL,
  runtime_pod_id BIGINT NULL,
  invocation_id INT NULL,
  event_type VARCHAR(100) NOT NULL,
  traffic_class VARCHAR(50) NOT NULL,
  severity VARCHAR(20) NOT NULL,
  message VARCHAR(500) NOT NULL,
  details LONGTEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_events_trace_id (trace_id),
  INDEX idx_audit_events_request_id (request_id),
  INDEX idx_audit_events_user_id (user_id),
  INDEX idx_audit_events_gateway_id (gateway_id),
  INDEX idx_audit_events_invocation_id (invocation_id),
  INDEX idx_audit_events_event_type (event_type),
  INDEX idx_audit_events_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS cost_records (
  id INT AUTO_INCREMENT PRIMARY KEY,
  trace_id VARCHAR(100) NOT NULL,
  session_id VARCHAR(100) NULL,
  request_id VARCHAR(100) NULL,
  user_id INT NULL,
  instance_id INT NULL,
  instance_mode VARCHAR(16) NULL,
  runtime_type VARCHAR(32) NULL,
  gateway_id VARCHAR(128) NULL,
  runtime_pod_id BIGINT NULL,
  invocation_id INT NULL,
  model_id INT NULL,
  provider_type VARCHAR(100) NOT NULL,
  model_name VARCHAR(255) NOT NULL,
  currency VARCHAR(16) NOT NULL DEFAULT 'USD',
  prompt_tokens INT NOT NULL DEFAULT 0,
  completion_tokens INT NOT NULL DEFAULT 0,
  total_tokens INT NOT NULL DEFAULT 0,
  input_unit_price DECIMAL(18,8) NOT NULL DEFAULT 0,
  output_unit_price DECIMAL(18,8) NOT NULL DEFAULT 0,
  estimated_cost DECIMAL(18,8) NOT NULL DEFAULT 0,
  actual_cost DECIMAL(18,8) NULL,
  internal_cost DECIMAL(18,8) NOT NULL DEFAULT 0,
  recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_cost_records_trace_id (trace_id),
  INDEX idx_cost_records_user_id (user_id),
  INDEX idx_cost_records_gateway_id (gateway_id),
  INDEX idx_cost_records_model_id (model_id),
  INDEX idx_cost_records_provider_type (provider_type),
  INDEX idx_cost_records_recorded_at (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS risk_hits (
  id INT AUTO_INCREMENT PRIMARY KEY,
  trace_id VARCHAR(100) NOT NULL,
  session_id VARCHAR(100) NULL,
  request_id VARCHAR(100) NULL,
  user_id INT NULL,
  instance_id INT NULL,
  instance_mode VARCHAR(16) NULL,
  runtime_type VARCHAR(32) NULL,
  gateway_id VARCHAR(128) NULL,
  runtime_pod_id BIGINT NULL,
  invocation_id INT NULL,
  rule_id VARCHAR(100) NOT NULL,
  rule_name VARCHAR(255) NOT NULL,
  severity VARCHAR(20) NOT NULL,
  action VARCHAR(50) NOT NULL,
  match_summary VARCHAR(500) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_risk_hits_trace_id (trace_id),
  INDEX idx_risk_hits_request_id (request_id),
  INDEX idx_risk_hits_user_id (user_id),
  INDEX idx_risk_hits_gateway_id (gateway_id),
  INDEX idx_risk_hits_invocation_id (invocation_id),
  INDEX idx_risk_hits_severity (severity),
  INDEX idx_risk_hits_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_invocations' AND COLUMN_NAME = 'instance_mode'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE model_invocations ADD COLUMN instance_mode VARCHAR(16) NULL AFTER instance_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_invocations' AND COLUMN_NAME = 'runtime_type'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE model_invocations ADD COLUMN runtime_type VARCHAR(32) NULL AFTER instance_mode', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_invocations' AND COLUMN_NAME = 'gateway_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE model_invocations ADD COLUMN gateway_id VARCHAR(128) NULL AFTER runtime_type', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_invocations' AND COLUMN_NAME = 'runtime_pod_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE model_invocations ADD COLUMN runtime_pod_id BIGINT NULL AFTER gateway_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_events' AND COLUMN_NAME = 'instance_mode'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE audit_events ADD COLUMN instance_mode VARCHAR(16) NULL AFTER instance_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_events' AND COLUMN_NAME = 'runtime_type'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE audit_events ADD COLUMN runtime_type VARCHAR(32) NULL AFTER instance_mode', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_events' AND COLUMN_NAME = 'gateway_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE audit_events ADD COLUMN gateway_id VARCHAR(128) NULL AFTER runtime_type', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_events' AND COLUMN_NAME = 'runtime_pod_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE audit_events ADD COLUMN runtime_pod_id BIGINT NULL AFTER gateway_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cost_records' AND COLUMN_NAME = 'instance_mode'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE cost_records ADD COLUMN instance_mode VARCHAR(16) NULL AFTER instance_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cost_records' AND COLUMN_NAME = 'runtime_type'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE cost_records ADD COLUMN runtime_type VARCHAR(32) NULL AFTER instance_mode', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cost_records' AND COLUMN_NAME = 'gateway_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE cost_records ADD COLUMN gateway_id VARCHAR(128) NULL AFTER runtime_type', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cost_records' AND COLUMN_NAME = 'runtime_pod_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE cost_records ADD COLUMN runtime_pod_id BIGINT NULL AFTER gateway_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_hits' AND COLUMN_NAME = 'instance_mode'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE risk_hits ADD COLUMN instance_mode VARCHAR(16) NULL AFTER instance_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_hits' AND COLUMN_NAME = 'runtime_type'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE risk_hits ADD COLUMN runtime_type VARCHAR(32) NULL AFTER instance_mode', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_hits' AND COLUMN_NAME = 'gateway_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE risk_hits ADD COLUMN gateway_id VARCHAR(128) NULL AFTER runtime_type', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @column_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_hits' AND COLUMN_NAME = 'runtime_pod_id'
);
SET @ddl = IF(@column_exists = 0, 'ALTER TABLE risk_hits ADD COLUMN runtime_pod_id BIGINT NULL AFTER gateway_id', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @index_exists = (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_invocations' AND INDEX_NAME = 'idx_model_invocations_gateway_id'
);
SET @ddl = IF(@index_exists = 0, 'ALTER TABLE model_invocations ADD INDEX idx_model_invocations_gateway_id (gateway_id)', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @index_exists = (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_events' AND INDEX_NAME = 'idx_audit_events_gateway_id'
);
SET @ddl = IF(@index_exists = 0, 'ALTER TABLE audit_events ADD INDEX idx_audit_events_gateway_id (gateway_id)', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @index_exists = (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cost_records' AND INDEX_NAME = 'idx_cost_records_gateway_id'
);
SET @ddl = IF(@index_exists = 0, 'ALTER TABLE cost_records ADD INDEX idx_cost_records_gateway_id (gateway_id)', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @index_exists = (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_hits' AND INDEX_NAME = 'idx_risk_hits_gateway_id'
);
SET @ddl = IF(@index_exists = 0, 'ALTER TABLE risk_hits ADD INDEX idx_risk_hits_gateway_id (gateway_id)', 'SELECT 1');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

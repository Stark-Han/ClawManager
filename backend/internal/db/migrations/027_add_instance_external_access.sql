CREATE TABLE IF NOT EXISTS instance_external_access (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  instance_id INT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 0,
  auth_mode VARCHAR(32) NOT NULL DEFAULT 'public_link',
  public_slug VARCHAR(64) NULL,
  public_token_hash VARCHAR(128) NULL,
  short_code_hash VARCHAR(128) NULL,
  api_key_hash VARCHAR(128) NULL,
  password_value VARCHAR(128) NULL,
  api_key_prefix VARCHAR(32) NULL,
  expires_at DATETIME NULL,
  created_by INT NULL,
  last_used_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_instance_external_access_instance (instance_id),
  UNIQUE KEY uk_instance_external_access_slug (public_slug),
  UNIQUE KEY uk_instance_external_access_short_code (short_code_hash),
  KEY idx_instance_external_access_enabled (enabled, auth_mode),
  CONSTRAINT fk_instance_external_access_instance
    FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

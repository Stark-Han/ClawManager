CREATE TABLE IF NOT EXISTS openclaw_config_bundle_skills (
  id INT AUTO_INCREMENT PRIMARY KEY,
  bundle_id INT NOT NULL,
  skill_id INT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  required BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (bundle_id) REFERENCES openclaw_config_bundles(id) ON DELETE CASCADE,
  FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE,
  UNIQUE KEY uk_openclaw_bundle_skill (bundle_id, skill_id),
  INDEX idx_openclaw_bundle_skill_bundle (bundle_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

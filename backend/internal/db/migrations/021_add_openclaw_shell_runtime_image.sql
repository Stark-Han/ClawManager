SET @system_image_is_enabled_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'system_image_settings'
    AND COLUMN_NAME = 'is_enabled'
);
SET @system_image_is_enabled_column_sql = IF(
  @system_image_is_enabled_column_exists = 0,
  'ALTER TABLE system_image_settings ADD COLUMN is_enabled BOOLEAN NOT NULL DEFAULT TRUE',
  'SELECT 1'
);
PREPARE system_image_is_enabled_column_stmt FROM @system_image_is_enabled_column_sql;
EXECUTE system_image_is_enabled_column_stmt;
DEALLOCATE PREPARE system_image_is_enabled_column_stmt;

SET @system_image_unique_instance_type_index_name = (
  SELECT INDEX_NAME
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'system_image_settings'
    AND COLUMN_NAME = 'instance_type'
    AND NON_UNIQUE = 0
    AND INDEX_NAME <> 'PRIMARY'
  LIMIT 1
);
SET @system_image_unique_instance_type_index_sql = IF(
  @system_image_unique_instance_type_index_name IS NOT NULL,
  CONCAT(
    'ALTER TABLE system_image_settings DROP INDEX `',
    REPLACE(@system_image_unique_instance_type_index_name, '`', '``'),
    '`'
  ),
  'SELECT 1'
);
PREPARE system_image_unique_instance_type_index_stmt FROM @system_image_unique_instance_type_index_sql;
EXECUTE system_image_unique_instance_type_index_stmt;
DEALLOCATE PREPARE system_image_unique_instance_type_index_stmt;

INSERT INTO system_image_settings (instance_type, runtime_type, display_name, image, is_enabled)
SELECT
  'openclaw',
  'desktop',
  'OpenClaw Desktop',
  'ghcr.io/yuan-lab-llm/agentsruntime/openclaw:latest',
  TRUE
WHERE NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'openclaw'
    AND runtime_type = 'desktop'
)
AND NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'openclaw'
    AND is_enabled = FALSE
);

INSERT INTO system_image_settings (instance_type, runtime_type, display_name, image, is_enabled)
SELECT
  'openclaw',
  'shell',
  'OpenClaw Lite',
  'ghcr.io/yuan-lab-llm/agentsruntime/openclaw-lite:latest',
  TRUE
WHERE NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'openclaw'
    AND runtime_type = 'shell'
)
AND NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'openclaw'
    AND is_enabled = FALSE
);

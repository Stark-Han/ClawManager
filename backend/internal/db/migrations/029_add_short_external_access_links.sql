SET @column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instance_external_access'
    AND COLUMN_NAME = 'short_code_hash'
);
SET @ddl = IF(
  @column_exists = 0,
  'ALTER TABLE instance_external_access ADD COLUMN short_code_hash VARCHAR(128) NULL AFTER public_token_hash',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @public_slug_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instance_external_access'
    AND COLUMN_NAME = 'public_slug'
);
SET @ddl = IF(
  @public_slug_exists > 0,
  'ALTER TABLE instance_external_access MODIFY COLUMN public_slug VARCHAR(64) NULL',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instance_external_access'
    AND INDEX_NAME = 'uk_instance_external_access_short_code'
);
SET @ddl = IF(
  @index_exists = 0,
  'ALTER TABLE instance_external_access ADD UNIQUE KEY uk_instance_external_access_short_code (short_code_hash)',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

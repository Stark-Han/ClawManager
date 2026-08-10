SET @column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instance_external_access'
    AND COLUMN_NAME = 'password_value'
);
SET @ddl = IF(
  @column_exists = 0,
  'ALTER TABLE instance_external_access ADD COLUMN password_value VARCHAR(128) NULL AFTER api_key_hash',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

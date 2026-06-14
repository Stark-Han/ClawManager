SET @instance_mode_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'instances'
    AND COLUMN_NAME = 'instance_mode'
);
SET @instance_mode_column_sql = IF(
  @instance_mode_column_exists = 0,
  'ALTER TABLE instances ADD COLUMN instance_mode ENUM(''lite'', ''pro'') NOT NULL DEFAULT ''lite'' AFTER runtime_type',
  'SELECT 1'
);
PREPARE instance_mode_column_stmt FROM @instance_mode_column_sql;
EXECUTE instance_mode_column_stmt;
DEALLOCATE PREPARE instance_mode_column_stmt;

UPDATE instances
SET instance_mode = CASE
  WHEN runtime_type = 'gateway' THEN 'lite'
  ELSE 'pro'
END
WHERE instance_mode IS NULL
  OR instance_mode = ''
  OR (runtime_type <> 'gateway' AND instance_mode = 'lite');

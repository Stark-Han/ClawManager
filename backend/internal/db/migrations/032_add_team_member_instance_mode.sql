SET @team_member_instance_mode_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'instance_mode'
);
SET @team_member_instance_mode_column_sql = IF(
  @team_member_instance_mode_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN instance_mode ENUM(''lite'', ''pro'') NOT NULL DEFAULT ''lite'' AFTER runtime_type',
  'SELECT 1'
);
PREPARE team_member_instance_mode_column_stmt FROM @team_member_instance_mode_column_sql;
EXECUTE team_member_instance_mode_column_stmt;
DEALLOCATE PREPARE team_member_instance_mode_column_stmt;

UPDATE team_members tm
LEFT JOIN instances i ON i.id = tm.instance_id
SET tm.instance_mode = CASE
  WHEN i.instance_mode IN ('lite', 'pro') THEN i.instance_mode
  WHEN i.runtime_type = 'gateway' THEN 'lite'
  WHEN i.id IS NOT NULL THEN 'pro'
  ELSE tm.instance_mode
END
WHERE tm.instance_mode IS NULL
  OR tm.instance_mode = ''
  OR (i.id IS NOT NULL AND tm.instance_mode <> COALESCE(i.instance_mode, CASE WHEN i.runtime_type = 'gateway' THEN 'lite' ELSE 'pro' END));

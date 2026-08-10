SET @team_member_availability_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'availability'
);
SET @team_member_availability_column_sql = IF(
  @team_member_availability_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN availability VARCHAR(30) NOT NULL DEFAULT ''unknown'' AFTER last_seen_at',
  'SELECT 1'
);
PREPARE team_member_availability_column_stmt FROM @team_member_availability_column_sql;
EXECUTE team_member_availability_column_stmt;
DEALLOCATE PREPARE team_member_availability_column_stmt;

SET @team_member_runtime_status_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'runtime_status'
);
SET @team_member_runtime_status_column_sql = IF(
  @team_member_runtime_status_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN runtime_status VARCHAR(50) NULL AFTER availability',
  'SELECT 1'
);
PREPARE team_member_runtime_status_column_stmt FROM @team_member_runtime_status_column_sql;
EXECUTE team_member_runtime_status_column_stmt;
DEALLOCATE PREPARE team_member_runtime_status_column_stmt;

SET @team_member_runtime_task_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'runtime_task_id'
);
SET @team_member_runtime_task_column_sql = IF(
  @team_member_runtime_task_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN runtime_task_id VARCHAR(255) NULL AFTER runtime_status',
  'SELECT 1'
);
PREPARE team_member_runtime_task_column_stmt FROM @team_member_runtime_task_column_sql;
EXECUTE team_member_runtime_task_column_stmt;
DEALLOCATE PREPARE team_member_runtime_task_column_stmt;

SET @team_member_runtime_intent_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'runtime_intent'
);
SET @team_member_runtime_intent_column_sql = IF(
  @team_member_runtime_intent_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN runtime_intent VARCHAR(100) NULL AFTER runtime_task_id',
  'SELECT 1'
);
PREPARE team_member_runtime_intent_column_stmt FROM @team_member_runtime_intent_column_sql;
EXECUTE team_member_runtime_intent_column_stmt;
DEALLOCATE PREPARE team_member_runtime_intent_column_stmt;

SET @team_member_blocked_reason_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'blocked_reason'
);
SET @team_member_blocked_reason_column_sql = IF(
  @team_member_blocked_reason_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN blocked_reason TEXT NULL AFTER runtime_intent',
  'SELECT 1'
);
PREPARE team_member_blocked_reason_column_stmt FROM @team_member_blocked_reason_column_sql;
EXECUTE team_member_blocked_reason_column_stmt;
DEALLOCATE PREPARE team_member_blocked_reason_column_stmt;

SET @team_member_last_summary_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'last_summary'
);
SET @team_member_last_summary_column_sql = IF(
  @team_member_last_summary_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN last_summary TEXT NULL AFTER blocked_reason',
  'SELECT 1'
);
PREPARE team_member_last_summary_column_stmt FROM @team_member_last_summary_column_sql;
EXECUTE team_member_last_summary_column_stmt;
DEALLOCATE PREPARE team_member_last_summary_column_stmt;

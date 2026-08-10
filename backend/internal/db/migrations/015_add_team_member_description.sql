SET @team_member_description_column_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'team_members'
    AND COLUMN_NAME = 'description'
);
SET @team_member_description_column_sql = IF(
  @team_member_description_column_exists = 0,
  'ALTER TABLE team_members ADD COLUMN description TEXT NULL AFTER role',
  'SELECT 1'
);
PREPARE team_member_description_column_stmt FROM @team_member_description_column_sql;
EXECUTE team_member_description_column_stmt;
DEALLOCATE PREPARE team_member_description_column_stmt;

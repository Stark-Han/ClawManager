UPDATE teams
SET name = CONCAT(
  LEFT(name, GREATEST(1, 255 - CHAR_LENGTH(CONCAT('__deleted_', id)))),
  '__deleted_',
  id
)
WHERE status = 'deleted'
  AND name NOT REGEXP '__deleted_[0-9]+$';

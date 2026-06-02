UPDATE instances
SET mount_path = '/config',
    updated_at = CURRENT_TIMESTAMP
WHERE type IN ('openclaw', 'ubuntu', 'webtop', 'hermes')
  AND mount_path IN ('/data', '/home/user/data', '/config/.hermes');

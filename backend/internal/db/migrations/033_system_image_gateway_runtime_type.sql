ALTER TABLE system_image_settings
  MODIFY COLUMN runtime_type ENUM('desktop', 'shell', 'gateway') NOT NULL DEFAULT 'desktop';

UPDATE system_image_settings
SET runtime_type = 'gateway',
    display_name = 'OpenClaw Lite'
WHERE instance_type = 'openclaw'
  AND runtime_type = 'shell';

UPDATE system_image_settings
SET runtime_type = 'gateway',
    display_name = 'Hermes Lite'
WHERE instance_type = 'hermes'
  AND runtime_type = 'shell';

UPDATE system_image_settings
SET display_name = 'OpenClaw Pro'
WHERE instance_type = 'openclaw'
  AND runtime_type = 'desktop'
  AND display_name IN ('OpenClaw Desktop', 'OpenClaw Runtime');

UPDATE system_image_settings
SET display_name = 'Hermes Pro'
WHERE instance_type = 'hermes'
  AND runtime_type = 'desktop'
  AND display_name IN ('Hermes Runtime', 'Hermes Desktop');

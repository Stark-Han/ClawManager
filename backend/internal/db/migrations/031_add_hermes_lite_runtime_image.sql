UPDATE system_image_settings
SET
  image = 'ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest',
  display_name = 'Hermes Lite',
  is_enabled = TRUE
WHERE instance_type = 'hermes'
  AND runtime_type = 'shell'
  AND image IN (
    'ghcr.io/yuan-lab-llm/agentsruntime/hermes-shell:latest',
    'registry.example.com/your-custom-shell-image:latest'
  );

INSERT INTO system_image_settings (instance_type, runtime_type, display_name, image, is_enabled)
SELECT
  'hermes',
  'shell',
  'Hermes Lite',
  'ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest',
  TRUE
WHERE NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'hermes'
    AND runtime_type = 'shell'
)
AND NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'hermes'
    AND is_enabled = FALSE
);

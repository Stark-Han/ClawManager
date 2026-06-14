UPDATE system_image_settings
SET
  image = 'ghcr.io/yuan-lab-llm/agentsruntime/openclaw-lite:latest',
  display_name = 'OpenClaw Lite',
  is_enabled = TRUE
WHERE instance_type = 'openclaw'
  AND runtime_type IN ('shell', 'gateway')
  AND image IN (
    'ghcr.io/yuan-lab-llm/agentsruntime/openclaw-shell:latest',
    '172.16.1.12:5010/openclaw-shell:local5',
    '172.16.1.12:5010/openclaw:v2dev'
  );

UPDATE system_image_settings
SET
  image = 'ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest',
  display_name = 'Hermes Lite',
  is_enabled = TRUE
WHERE instance_type = 'hermes'
  AND runtime_type IN ('shell', 'gateway')
  AND image IN (
    'ghcr.io/yuan-lab-llm/agentsruntime/hermes-shell:latest',
    '172.16.1.12:5010/hermes:codex-shared-agent-tui-bundle-20260609153019',
    '172.16.1.12:5010/hermes:team-lite-tui-dist-20260612174205',
    'registry.example.com/your-custom-shell-image:latest'
  );

INSERT INTO system_image_settings (instance_type, runtime_type, display_name, image, is_enabled)
SELECT
  'openclaw',
  'gateway',
  'OpenClaw Lite',
  'ghcr.io/yuan-lab-llm/agentsruntime/openclaw-lite:latest',
  TRUE
WHERE NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'openclaw'
    AND runtime_type IN ('shell', 'gateway')
);

INSERT INTO system_image_settings (instance_type, runtime_type, display_name, image, is_enabled)
SELECT
  'hermes',
  'gateway',
  'Hermes Lite',
  'ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest',
  TRUE
WHERE NOT EXISTS (
  SELECT 1
  FROM system_image_settings
  WHERE instance_type = 'hermes'
    AND runtime_type IN ('shell', 'gateway')
);

-- Add chrome_portal automation type + GFX homepage capture task
ALTER TABLE tasks MODIFY automation_type
  ENUM('http','chrome_extension','chrome_hybrid','cred_fetch','chrome_portal')
  NOT NULL DEFAULT 'http';

INSERT INTO tasks (task_uid, task_name, website_group, automation_type, interval_minutes, is_enabled)
VALUES ('gfx_captureHomepage', 'GFX Portal Homepage (local cookies)', 'gfx', 'chrome_portal', 60, 0)
ON DUPLICATE KEY UPDATE
  task_name = VALUES(task_name),
  website_group = VALUES(website_group),
  automation_type = VALUES(automation_type),
  interval_minutes = VALUES(interval_minutes);

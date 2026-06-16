-- GoHttpAuto Database Schema
-- Database: newhttpgoauto
-- Run via phpMyAdmin on server 74.208.99.161

SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
SET time_zone = "+00:00";

-- ─── Users (master + operator/friend accounts) ───────────────────────────────
CREATE TABLE IF NOT EXISTS `users` (
  `id`            INT AUTO_INCREMENT PRIMARY KEY,
  `username`      VARCHAR(100) NOT NULL UNIQUE,
  `password_hash` VARCHAR(255) NOT NULL,
  `role`          ENUM('master','operator') NOT NULL DEFAULT 'operator',
  `display_name`  VARCHAR(100) DEFAULT NULL,
  `is_active`     TINYINT(1) NOT NULL DEFAULT 1,
  `last_login`    DATETIME DEFAULT NULL,
  `created_by`    VARCHAR(100) DEFAULT NULL,
  `created_at`    DATETIME DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── API keys (external servers trigger automations) ─────────────────────────
CREATE TABLE IF NOT EXISTS `api_keys` (
  `id`          INT AUTO_INCREMENT PRIMARY KEY,
  `label`       VARCHAR(100) NOT NULL,
  `key_hash`    VARCHAR(255) NOT NULL,
  `key_prefix`  VARCHAR(12) NOT NULL,
  `is_active`   TINYINT(1) NOT NULL DEFAULT 1,
  `created_at`  DATETIME DEFAULT CURRENT_TIMESTAMP,
  `last_used`   DATETIME DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Automation tasks ────────────────────────────────────────────────────────
-- automation_type:
--   http             = pure HTTP (no browser)
--   chrome_extension = needs Chrome + extension
--   chrome_hybrid    = HTTP login + Chrome extension step
--   cred_fetch       = only scrape/fetch credentials
CREATE TABLE IF NOT EXISTS `tasks` (
  `task_uid`          VARCHAR(100) NOT NULL PRIMARY KEY,
  `task_name`         VARCHAR(150) NOT NULL,
  `website_group`     VARCHAR(50)  NOT NULL,
  `automation_type`   ENUM('http','chrome_extension','chrome_hybrid','cred_fetch') NOT NULL DEFAULT 'http',
  `interval_minutes`  INT NOT NULL DEFAULT 60,
  `is_enabled`        TINYINT(1) NOT NULL DEFAULT 0,
  `last_run_at`       DATETIME DEFAULT NULL,
  `next_run_at`       DATETIME DEFAULT NULL,
  `created_at`        DATETIME DEFAULT CURRENT_TIMESTAMP,
  `updated_at`        DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_tasks_group` (`website_group`),
  INDEX `idx_tasks_enabled` (`is_enabled`),
  INDEX `idx_tasks_next_run` (`next_run_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Portal credentials (read by automation engine) ────────────────────────
CREATE TABLE IF NOT EXISTS `credentials` (
  `website_id`    VARCHAR(100) NOT NULL PRIMARY KEY,
  `label`         VARCHAR(150) DEFAULT NULL,
  `username`      VARCHAR(255) NOT NULL,
  `password_enc`  TEXT NOT NULL,
  `is_enabled`    TINYINT(1) NOT NULL DEFAULT 1,
  `updated_at`    DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `created_at`    DATETIME DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Captured sessions (cookies, localStorage, etc.) ─────────────────────────
CREATE TABLE IF NOT EXISTS `shared_sessions` (
  `website_id`        VARCHAR(100) NOT NULL PRIMARY KEY,
  `cookies_json`      LONGTEXT,
  `cookies_netscape`  LONGTEXT,
  `cookies_header`    LONGTEXT,
  `local_storage`     LONGTEXT,
  `indexed_db`        LONGTEXT,
  `updated_at`        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `created_at`        DATETIME DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Task execution logs (auto-delete after 2 days) ──────────────────────────
CREATE TABLE IF NOT EXISTS `task_logs` (
  `id`           INT AUTO_INCREMENT PRIMARY KEY,
  `task_uid`     VARCHAR(100) NOT NULL,
  `status`       ENUM('running','success','failed') NOT NULL,
  `message`      TEXT,
  `triggered_by` VARCHAR(100) DEFAULT 'cron',
  `duration_ms`  INT NOT NULL DEFAULT 0,
  `created_at`   DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_logs_task` (`task_uid`),
  INDEX `idx_logs_status` (`status`),
  INDEX `idx_logs_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Scraped credentials (GFX cred-fetch automations) ──────────────────────
CREATE TABLE IF NOT EXISTS `scraped_credentials` (
  `id`               INT AUTO_INCREMENT PRIMARY KEY,
  `source_platform`  VARCHAR(50) NOT NULL,
  `website_name`     VARCHAR(100) NOT NULL,
  `login_url`        TEXT,
  `username`         VARCHAR(255) NOT NULL,
  `password`         TEXT NOT NULL,
  `updated_at`       DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `created_at`       DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY `uq_scraped` (`source_platform`,`website_name`,`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Concurrency locks ───────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `system_locks` (
  `lock_key`   VARCHAR(100) NOT NULL PRIMARY KEY,
  `lock_state` INT NOT NULL DEFAULT 0,
  `locked_by`  VARCHAR(100) DEFAULT NULL,
  `locked_at`  DATETIME DEFAULT NULL,
  `expires_at` DATETIME DEFAULT NULL,
  INDEX `idx_locks_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Audit trail ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `activity_logs` (
  `id`         INT AUTO_INCREMENT PRIMARY KEY,
  `username`   VARCHAR(100) NOT NULL,
  `role`       VARCHAR(20) NOT NULL,
  `action`     VARCHAR(100) NOT NULL,
  `details`    TEXT,
  `ip_address` VARCHAR(45) DEFAULT NULL,
  `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_activity_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Job queue (panel enqueues → worker Mac executes) ────────────────────────
CREATE TABLE IF NOT EXISTS `job_queue` (
  `id`           INT AUTO_INCREMENT PRIMARY KEY,
  `task_uid`     VARCHAR(100) NOT NULL,
  `triggered_by` VARCHAR(100) NOT NULL DEFAULT 'manual',
  `status`       ENUM('pending','claimed','done') NOT NULL DEFAULT 'pending',
  `claimed_by`   VARCHAR(100) DEFAULT NULL,
  `claimed_at`   DATETIME DEFAULT NULL,
  `finished_at`  DATETIME DEFAULT NULL,
  `created_at`   DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_job_status` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── Seed: HTTP automation tasks ───────────────────────────────────────────────
INSERT INTO `tasks` (`task_uid`, `task_name`, `website_group`, `automation_type`, `interval_minutes`, `is_enabled`) VALUES
('nox_runSemrush', 'Semrush (NoxTools)', 'nox', 'http', 20, 0),
('azad_runAzadSemrush', 'Semrush (Azad)', 'azad', 'http', 60, 0),
('toolbaazar_runToolbaazarSemrush', 'Semrush (Toolbaazar)', 'toolbaazar', 'http', 60, 0)
ON DUPLICATE KEY UPDATE task_name = VALUES(task_name);

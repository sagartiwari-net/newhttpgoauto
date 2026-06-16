-- Run once in phpMyAdmin on database newhttpgoauto
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

-- Extend job_queue status for failed/cancelled jobs
ALTER TABLE `job_queue`
  MODIFY `status` ENUM('pending','claimed','done','failed','cancelled') NOT NULL DEFAULT 'pending';

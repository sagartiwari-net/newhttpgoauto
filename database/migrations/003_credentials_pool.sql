-- GFX dynamic account pool (run once on MySQL)
ALTER TABLE `credentials`
  ADD COLUMN `pool_group` VARCHAR(50) DEFAULT NULL COMMENT 'e.g. gfxtoolz',
  ADD COLUMN `pool_role` ENUM('default','scraper') NOT NULL DEFAULT 'default';

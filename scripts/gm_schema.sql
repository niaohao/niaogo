-- GM 后台增量 DDL（2026-07-25）
-- 在已有 niao 库上执行；与 说明/06-GM后台规划.md 第五节对齐
USE niao;

-- users 扩展
ALTER TABLE users
  ADD COLUMN last_login_ip VARCHAR(64) NOT NULL DEFAULT '' AFTER login_streak,
  ADD COLUMN register_device VARCHAR(128) NOT NULL DEFAULT '' AFTER last_login_ip,
  ADD COLUMN ban_flag TINYINT NOT NULL DEFAULT 0 COMMENT '0正常 1封禁缓存' AFTER register_device;

-- 若上列已存在会报错，可忽略或逐列检查后执行

CREATE TABLE IF NOT EXISTS user_login_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  ip VARCHAR(64) NOT NULL DEFAULT '',
  device VARCHAR(128) NOT NULL DEFAULT '',
  success TINYINT NOT NULL DEFAULT 1,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at DESC),
  INDEX idx_ip_time (ip, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS gm_accounts (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(128) NOT NULL COMMENT 'bcrypt/argon2',
  role VARCHAR(16) NOT NULL DEFAULT 'trainee' COMMENT 'super|gm|trainee',
  display_name VARCHAR(64) NOT NULL DEFAULT '',
  enabled TINYINT NOT NULL DEFAULT 1,
  limits_json JSON NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  last_login_at TIMESTAMP NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS gm_bans (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  ban_type VARCHAR(16) NOT NULL COMMENT 'temp|perm',
  reason VARCHAR(512) NOT NULL,
  banned_by VARCHAR(64) NOT NULL,
  start_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  end_at DATETIME NULL COMMENT '永久则 NULL',
  active TINYINT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_active (user_id, active),
  INDEX idx_end (end_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS coins_ledger (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  delta INT NOT NULL,
  balance_after INT NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  ref_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_time (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS gm_server_flags (
  flag_key VARCHAR(64) PRIMARY KEY,
  flag_value VARCHAR(256) NOT NULL DEFAULT '',
  updated_by VARCHAR(64) NOT NULL DEFAULT '',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO gm_server_flags (flag_key, flag_value) VALUES
  ('exp_double', '0'),
  ('drop_double', '0'),
  ('maintain', '0');

-- gm_audit 增强（列已存在则跳过）
ALTER TABLE gm_audit
  ADD COLUMN gm_role VARCHAR(16) NOT NULL DEFAULT '' AFTER admin,
  ADD COLUMN client_ip VARCHAR(64) NOT NULL DEFAULT '' AFTER detail_json,
  ADD COLUMN danger TINYINT NOT NULL DEFAULT 0 AFTER client_ip;

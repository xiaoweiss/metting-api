-- 全员群发的 cron + 模板 配置（区别于已有 email_schedules，那个是按 group 发；这个是全员）
-- 单行表（lock_key 是固定字符串 'singleton'，保证同一时刻只有一行配置）

CREATE TABLE IF NOT EXISTS mail_blast_schedules (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  lock_key    VARCHAR(32) NOT NULL DEFAULT 'singleton',
  cron_expr   VARCHAR(64) NOT NULL DEFAULT '0 0 8 * * *',
  template_id BIGINT NOT NULL DEFAULT 0,
  enabled     BOOLEAN NOT NULL DEFAULT FALSE,
  last_run_at DATETIME DEFAULT NULL COMMENT '上一次实际触发的时间，用于防止 cron 抖动重复发',
  updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_lock_key (lock_key)
);

INSERT INTO mail_blast_schedules (lock_key, cron_expr, template_id, enabled)
VALUES ('singleton', '0 0 8 * * *', 0, FALSE)
ON DUPLICATE KEY UPDATE id = id;

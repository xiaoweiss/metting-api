-- 每条 email_logs 展开成 N 行 recipients，让后台能看到「这次群发到底发给了谁、每个人状态」
-- 重发单封时按 recipient_id 引用，retry_count 在 recipient 上维护，不影响母 log

CREATE TABLE IF NOT EXISTS email_log_recipients (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  log_id      BIGINT NOT NULL,
  email       VARCHAR(128) NOT NULL,
  status      ENUM('sent','failed') NOT NULL,
  error       TEXT,
  retry_count INT NOT NULL DEFAULT 0,
  sent_at     DATETIME NOT NULL,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  KEY idx_log_id  (log_id),
  KEY idx_log_status (log_id, status),
  KEY idx_email   (email)
);

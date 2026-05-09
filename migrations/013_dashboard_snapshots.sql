-- 看板截图留存表: PC 端用户点「保存」时同步上传一份,
-- 群发邮件时按 (hotel_id, date, mode='occupancy', format='png') 取来 inline 嵌入。
-- UNIQUE 索引允许同一酒店同一天同一模式同一 format 反复保存覆盖最新版(UPSERT)。

CREATE TABLE IF NOT EXISTS dashboard_snapshots (
  id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  hotel_id        BIGINT UNSIGNED NOT NULL,
  snapshot_date   DATE NOT NULL,
  mode            VARCHAR(16) NOT NULL,           -- 'occupancy' | 'bookings'
  format          VARCHAR(8)  NOT NULL,           -- 'png' | 'pdf'
  file_path       VARCHAR(255) NOT NULL,          -- 相对路径,e.g. 2026/05/dashboard-3-2026-05-09-occupancy.png
  file_size       INT UNSIGNED NOT NULL,
  uploaded_by     BIGINT UNSIGNED NULL,           -- users.id, 可空
  uploaded_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_hotel_date_mode_format (hotel_id, snapshot_date, mode, format),
  INDEX idx_uploaded_at (uploaded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 扩 email_log_recipients.status 枚举,补 'skipped'(snapshot 缺失时跳过该 recipient)
ALTER TABLE email_log_recipients
  MODIFY COLUMN status ENUM('sent','failed','skipped') NOT NULL;

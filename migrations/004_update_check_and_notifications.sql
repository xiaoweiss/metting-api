-- 004_update_check_and_notifications.sql
-- 新增：酒店每日更新检测日志 + 通知渠道配置

-- ========== 通知渠道配置 ==========
CREATE TABLE IF NOT EXISTS notification_settings (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  channel    VARCHAR(32)  NOT NULL   COMMENT 'sms / feishu / dingtalk_ding',
  config     JSON         NULL       COMMENT '渠道配置 (webhook url / access key / agent id 等)',
  enabled    TINYINT(1)   NOT NULL DEFAULT 0,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_channel (channel)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 初始化三条默认（关闭状态）
INSERT INTO notification_settings (channel, config, enabled) VALUES
  ('feishu',         JSON_OBJECT('webhookUrl', '', 'secret', ''), 0),
  ('sms',            JSON_OBJECT('accessKeyId', '', 'accessKeySecret', '', 'signName', '', 'templateCode', ''), 0),
  ('dingtalk_ding',  JSON_OBJECT('agentId', 0, 'useAppToken', true), 0)
ON DUPLICATE KEY UPDATE channel = channel;

-- ========== 每日更新检测日志 ==========
CREATE TABLE IF NOT EXISTS hotel_update_checks (
  id                 BIGINT       NOT NULL AUTO_INCREMENT,
  check_date         DATE         NOT NULL   COMMENT '检测针对的业务日期',
  hotel_id           BIGINT       NOT NULL,
  is_updated         TINYINT(1)   NOT NULL   COMMENT '当日是否有数据录入',
  record_count       INT          NOT NULL DEFAULT 0 COMMENT '当日 meeting_records 行数',
  notified_channels  VARCHAR(128) NULL       COMMENT '已通知渠道（逗号分隔）',
  notified_at        DATETIME     NULL       COMMENT '通知时间',
  created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_date_hotel (check_date, hotel_id),
  KEY idx_check_date (check_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ========== 用户电话（用于 SMS） ==========
ALTER TABLE users
  ADD COLUMN phone VARCHAR(32) NULL DEFAULT NULL COMMENT '手机号 用于 SMS 通知' AFTER email;

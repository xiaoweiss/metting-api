-- 017_audit_logs.sql
-- 后台管理操作审计日志: users / mail_settings / email_groups / email_group_members / mail_templates / mail_blast_schedules
-- 失败不阻塞业务 (helper 内部捕获), 仅作回溯使用
CREATE TABLE audit_logs (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id      BIGINT       DEFAULT NULL COMMENT '操作人 user_id, NULL=system',
  user_name    VARCHAR(64)  DEFAULT NULL COMMENT '冗余存名字, 避免 JOIN',
  action       VARCHAR(32)  NOT NULL     COMMENT 'create / update / delete / trigger',
  target_type  VARCHAR(64)  NOT NULL     COMMENT 'users / email_groups / mail_settings / mail_templates / mail_blast_schedules / email_group_members',
  target_id    BIGINT       DEFAULT NULL COMMENT '对象 PK',
  target_name  VARCHAR(255) DEFAULT NULL COMMENT '冗余存对象名(group/template/hotel name)便于阅读',
  before_value JSON         DEFAULT NULL COMMENT '改前快照(整行 JSON)',
  after_value  JSON         DEFAULT NULL COMMENT '改后快照',
  ip           VARCHAR(64)  DEFAULT NULL COMMENT '操作来源 IP',
  created_at   DATETIME     DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_target (target_type, target_id),
  INDEX idx_user_created (user_id, created_at),
  INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 让"未录入数据"提醒的 cron 也能后台改，跟 sync_schedules 同款结构。
-- 第一行用 yaml 当前默认（每天 20:00），后台改了之后由 update_check_schedules 接管，
-- 启动时优先读 DB；DB 没记录或 enabled=0 时 fallback 回 yaml。

CREATE TABLE IF NOT EXISTS update_check_schedules (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  cron_expr  VARCHAR(64) NOT NULL,
  enabled    BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

INSERT INTO update_check_schedules (cron_expr, enabled)
SELECT '0 20 * * *', TRUE
WHERE NOT EXISTS (SELECT 1 FROM update_check_schedules);

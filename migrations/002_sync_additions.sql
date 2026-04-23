-- 002: Add sync-related schema changes

ALTER TABLE venues ADD COLUMN available_periods VARCHAR(64) DEFAULT NULL AFTER type;

ALTER TABLE meeting_records
  ADD COLUMN banquet_food_revenue  DECIMAL(12,2) DEFAULT 0 AFTER activity_type,
  ADD COLUMN banquet_venue_revenue DECIMAL(12,2) DEFAULT 0 AFTER banquet_food_revenue,
  ADD COLUMN entry_date DATE DEFAULT NULL AFTER banquet_venue_revenue;

CREATE TABLE IF NOT EXISTS sync_schedules (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  cron_expr VARCHAR(64) NOT NULL DEFAULT '0 6 * * *',
  enabled   TINYINT(1) DEFAULT 1,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
INSERT INTO sync_schedules (cron_expr, enabled) VALUES ('0 6 * * *', 1);

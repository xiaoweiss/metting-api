-- 会议室运营平台数据库初始化
CREATE DATABASE IF NOT EXISTS meeting_room_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE meeting_room_db;

CREATE TABLE IF NOT EXISTS users (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  dingtalk_union_id VARCHAR(64) UNIQUE NOT NULL,
  name              VARCHAR(64),
  email             VARCHAR(128),
  status            ENUM('pending','active','disabled') DEFAULT 'pending',
  is_admin          TINYINT(1) DEFAULT 0,
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS market_areas (
  id   BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL,
  city VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS hotels (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  name           VARCHAR(128) NOT NULL,
  city           VARCHAR(64),
  market_area_id BIGINT,
  created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS venues (
  id       BIGINT PRIMARY KEY AUTO_INCREMENT,
  hotel_id BIGINT NOT NULL,
  name     VARCHAR(128) NOT NULL,
  type     VARCHAR(64),
  INDEX idx_hotel (hotel_id)
);

CREATE TABLE IF NOT EXISTS user_hotel_perms (
  user_id  BIGINT NOT NULL,
  hotel_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, hotel_id)
);

CREATE TABLE IF NOT EXISTS meeting_records (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  hotel_id      BIGINT NOT NULL,
  venue_id      BIGINT NOT NULL,
  record_date   DATE NOT NULL,
  period        ENUM('AM','PM','EV') NOT NULL,
  is_booked     TINYINT(1) DEFAULT 0,
  activity_type VARCHAR(64),
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_record (hotel_id, venue_id, record_date, period),
  INDEX idx_date_hotel (record_date, hotel_id)
);

CREATE TABLE IF NOT EXISTS competitor_groups (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  name          VARCHAR(128) NOT NULL,
  base_hotel_id BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS competitor_group_hotels (
  group_id BIGINT NOT NULL,
  hotel_id BIGINT NOT NULL,
  PRIMARY KEY (group_id, hotel_id)
);

CREATE TABLE IF NOT EXISTS city_events (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  city       VARCHAR(64) NOT NULL,
  venue_name VARCHAR(128),
  event_name VARCHAR(256) NOT NULL,
  event_type VARCHAR(64),
  event_date DATE NOT NULL,
  INDEX idx_city_date (city, event_date)
);

CREATE TABLE IF NOT EXISTS color_thresholds (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  hotel_id    BIGINT,
  metric_type ENUM('occupancy','activity') NOT NULL,
  level       ENUM('low','medium','high') NOT NULL,
  min_value   DECIMAL(5,2) DEFAULT 0,
  max_value   DECIMAL(5,2) DEFAULT 0,
  color       VARCHAR(16) NOT NULL,
  UNIQUE KEY uk_threshold (hotel_id, metric_type, level)
);

-- 默认阈值（hotel_id=NULL 表示全局默认）
INSERT IGNORE INTO color_thresholds (hotel_id, metric_type, level, min_value, max_value, color)
VALUES
  (NULL,'occupancy','low',   0,   60,  '#FFA39E'),
  (NULL,'occupancy','medium',60,  80,  '#FFD591'),
  (NULL,'occupancy','high',  80,  100, '#B7EB8F'),
  (NULL,'activity', 'low',   0,   3,   '#FFA39E'),
  (NULL,'activity', 'medium',3,   7,   '#FFD591'),
  (NULL,'activity', 'high',  7,   999, '#B7EB8F');

CREATE TABLE IF NOT EXISTS email_groups (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  name       VARCHAR(128) NOT NULL,
  hotel_id   BIGINT,
  scene      VARCHAR(64),
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS email_group_members (
  group_id BIGINT NOT NULL,
  user_id  BIGINT NOT NULL,
  email    VARCHAR(128) NOT NULL,
  PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS email_schedules (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id  BIGINT NOT NULL,
  cron_expr VARCHAR(64) NOT NULL,
  enabled   TINYINT(1) DEFAULT 1,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS email_logs (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  schedule_id BIGINT NOT NULL,
  status      ENUM('success','partial','failed') NOT NULL,
  total       INT DEFAULT 0,
  fail_count  INT DEFAULT 0,
  fail_list   JSON,
  retry_count INT DEFAULT 0,
  sent_at     DATETIME,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_schedule (schedule_id),
  INDEX idx_created (created_at)
);

CREATE TABLE IF NOT EXISTS sync_logs (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  source       VARCHAR(64) NOT NULL,
  status       ENUM('success','failed') NOT NULL,
  record_count INT DEFAULT 0,
  message      TEXT,
  synced_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_synced (synced_at)
);

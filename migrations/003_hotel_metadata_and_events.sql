-- 003_hotel_metadata_and_events.sql
-- 给 hotels / venues 加元数据列（来自 酒店设施表 hAi1ytw）
-- 新增 hotel_events 表（来自 Hotel Event 表 zJSnWZm）

-- ========== hotels 元数据 ==========
ALTER TABLE hotels
  ADD COLUMN star_rating        VARCHAR(16)  NULL DEFAULT NULL COMMENT '星级 5-Star',
  ADD COLUMN brand              VARCHAR(64)  NULL DEFAULT NULL COMMENT '品牌',
  ADD COLUMN hotel_group        VARCHAR(64)  NULL DEFAULT NULL COMMENT '集团',
  ADD COLUMN region             VARCHAR(64)  NULL DEFAULT NULL COMMENT '地区 华东',
  ADD COLUMN province           VARCHAR(64)  NULL DEFAULT NULL COMMENT '省份',
  ADD COLUMN metropolitan_area  VARCHAR(128) NULL DEFAULT NULL COMMENT '城市群',
  ADD COLUMN core_city          VARCHAR(64)  NULL DEFAULT NULL COMMENT '城市群核心城市',
  ADD COLUMN hotel_type         VARCHAR(64)  NULL DEFAULT NULL COMMENT '酒店类型 Convention',
  ADD COLUMN business_district  VARCHAR(128) NULL DEFAULT NULL COMMENT '所属商圈（与 market_area 分开，用于更细粒度的商圈归属）';

-- ========== venues 元数据 ==========
ALTER TABLE venues
  ADD COLUMN area_sqm         DECIMAL(10,2) NULL DEFAULT NULL COMMENT '面积 平米',
  ADD COLUMN theater_capacity INT           NULL DEFAULT NULL COMMENT '剧院式容纳人数',
  ADD COLUMN has_pillar       TINYINT(1)    NULL DEFAULT NULL COMMENT '是否有柱';

-- ========== hotel_events 新表 ==========
CREATE TABLE IF NOT EXISTS hotel_events (
  id                  BIGINT       NOT NULL AUTO_INCREMENT,
  hotel_id            BIGINT       NOT NULL,
  venue_id            BIGINT       NOT NULL DEFAULT 0,
  event_name          VARCHAR(256) NULL DEFAULT NULL  COMMENT '活动名称 Event_Name',
  event_type          VARCHAR(64)  NULL DEFAULT NULL  COMMENT '活动类型 Event_Type',
  event_date          DATE         NOT NULL           COMMENT '活动日期 Event_Date',
  target_date         DATE         NULL DEFAULT NULL  COMMENT '预定日期 Target_Date',
  end_date            DATE         NULL DEFAULT NULL  COMMENT '结束日期 End_Date',
  period              VARCHAR(16)  NULL DEFAULT NULL  COMMENT 'AM/PM/EV, 多个用逗号',
  booking_status      VARCHAR(32)  NULL DEFAULT NULL  COMMENT '已出租 / 预订中',
  dingtalk_record_id  VARCHAR(32)  NULL DEFAULT NULL  COMMENT '钉钉 recordId 去重键',
  created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_dingtalk_record (dingtalk_record_id),
  KEY idx_hotel_date (hotel_id, event_date),
  KEY idx_venue (venue_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

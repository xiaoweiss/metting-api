-- 解决：钉钉「酒店会议室信息表」中同一酒店里同名但不同 type 的多行
-- 在我们 venues 表里被合并成一行（FirstOrCreate by hotel_id+name，最后写入的 type 胜出）。
--
-- 修法：用钉钉行 ID（record id）作为唯一键，sync_venues 改成按 record id 去重，
-- sync_records 改成按 linked record id 精确定位 venue（不再看 name）。
--
-- 现存数据 dingtalk_record_id 是 NULL，第一次同步会回填。
-- 同步把所有 venue 行 upsert 一遍后，被覆盖的同名 venue 会重新创建出独立行。

ALTER TABLE venues
  ADD COLUMN dingtalk_record_id VARCHAR(32) DEFAULT NULL,
  ADD UNIQUE INDEX uk_venue_dingtalk_record_id (dingtalk_record_id);

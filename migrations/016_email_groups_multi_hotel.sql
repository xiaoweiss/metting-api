-- 016: email_groups 改造支持多酒店 — hotel_id (单值) → hotel_ids (JSON 数组)
-- 一封模板针对多家酒店时，按 hotel_ids 循环各发一份(每家渲染各自数据)
-- 老数据迁移规则:
--   hotel_id IS NULL  → hotel_ids = []
--   hotel_id = 0      → hotel_ids = []  (历史"全员/无关"语义,不应作为有效 hotel_id)
--   hotel_id > 0      → hotel_ids = [hotel_id]
-- 按铁规 2: 直接 DROP hotel_id, 不留向前兼容字段

-- 1. 加可空列
ALTER TABLE email_groups ADD COLUMN hotel_ids JSON;

-- 2. 老数据迁移
UPDATE email_groups SET hotel_ids = JSON_ARRAY(hotel_id) WHERE hotel_id IS NOT NULL AND hotel_id > 0;
UPDATE email_groups SET hotel_ids = JSON_ARRAY() WHERE hotel_id IS NULL OR hotel_id = 0;

-- 3. NOT NULL 约束
ALTER TABLE email_groups MODIFY COLUMN hotel_ids JSON NOT NULL;

-- 4. 删除旧 hotel_id
ALTER TABLE email_groups DROP COLUMN hotel_id;

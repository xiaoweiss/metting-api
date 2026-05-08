-- 给 users 加「所属酒店」字段。
-- 区别于 user_hotel_perms（多对多权限）：
--   user_hotel_perms = 我能看哪些酒店的数据（权限）
--   primary_hotel_id = 我代表哪家酒店（日报渲染、个人订阅默认对标的酒店）
-- 不在多对多关系里也允许设（弹性，由前端只列出关联酒店给运营选）

ALTER TABLE users
  ADD COLUMN primary_hotel_id BIGINT DEFAULT NULL AFTER role_id,
  ADD KEY idx_primary_hotel_id (primary_hotel_id);

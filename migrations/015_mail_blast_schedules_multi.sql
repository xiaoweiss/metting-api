-- 全员群发改成多条任务支持(可加可删,各自独立 cron + 模板)。
-- 老的 1 行 singleton 数据保留,迁移成 name='默认日报'。

-- 1) 加 name 字段
ALTER TABLE mail_blast_schedules
  ADD COLUMN name VARCHAR(64) NOT NULL DEFAULT '' AFTER id;

-- 2) 给老的 singleton 行赋个名字
UPDATE mail_blast_schedules SET name = '默认日报' WHERE name = '' OR name = 'singleton';

-- 3) 移除 singleton 唯一约束 + lock_key 字段
ALTER TABLE mail_blast_schedules DROP INDEX uk_lock_key;
ALTER TABLE mail_blast_schedules DROP COLUMN lock_key;

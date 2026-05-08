-- email_logs 加 template_id 和 source 列：
--   - template_id：让重发能直接拿到模板 id（不用从 schedule_id 反推）
--   - source：标记来源 'blast' / 'group:<id>' / 'manual'，前端日志页好做"哪发的"展示

ALTER TABLE email_logs
  ADD COLUMN template_id BIGINT NOT NULL DEFAULT 0 AFTER schedule_id,
  ADD COLUMN source      VARCHAR(64) NOT NULL DEFAULT '' AFTER template_id;

-- 历史数据没有 template_id / source，标记为 'legacy' 让前端不会误以为可重发
UPDATE email_logs SET source = 'legacy' WHERE source = '';

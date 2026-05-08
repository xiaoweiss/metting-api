-- 为通知模块新增「email」渠道的配置行（开启即生效）
-- SMTP 凭证不在这里存，复用 mail_settings（后台已配置过的发件箱）。
-- config 字段保留为空 JSON，预留以后做"只发给某子集邮箱"等开关。

INSERT INTO notification_settings (channel, config, enabled, updated_at)
VALUES ('email', '{}', 1, NOW())
ON DUPLICATE KEY UPDATE
  enabled = VALUES(enabled),
  updated_at = VALUES(updated_at);

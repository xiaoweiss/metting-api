-- 005_rename_feishu_to_dingtalk_robot.sql
-- 弃用飞书机器人渠道，替换为钉钉群自定义机器人。
-- 旧行改 channel 即可（都是 webhook + 加签模型），但旧 URL/secret 对钉钉无意义，重置为空并关闭。

UPDATE notification_settings
SET channel = 'dingtalk_robot',
    config  = JSON_OBJECT('webhookUrl', '', 'secret', ''),
    enabled = 0
WHERE channel = 'feishu';

-- 如果之前没有飞书行（例如新库），保证有一条默认关闭的钉钉机器人配置
INSERT IGNORE INTO notification_settings (channel, config, enabled)
VALUES ('dingtalk_robot', JSON_OBJECT('webhookUrl', '', 'secret', ''), 0);

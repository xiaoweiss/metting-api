-- 邮件模板静态附件:每个模板可挂 N 个文件(图/PDF),群发该模板时全员收到同一份。
-- 跟 dashboard_snapshots(per-recipient 动态) 正交。

CREATE TABLE IF NOT EXISTS mail_template_attachments (
  id            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  template_id   BIGINT UNSIGNED NOT NULL,
  original_name VARCHAR(255) NOT NULL,        -- 原始文件名(展示用)
  file_path     VARCHAR(255) NOT NULL,        -- 相对路径 e.g. template-3/poster.png
  file_size     INT UNSIGNED NOT NULL,
  mime_type     VARCHAR(64) NOT NULL,         -- image/png, application/pdf, ...
  cid           VARCHAR(128) NOT NULL,        -- 邮件 Content-ID(同模板内唯一)
  sort_order    INT NOT NULL DEFAULT 0,
  uploaded_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_template_id (template_id),
  UNIQUE KEY uk_template_cid (template_id, cid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

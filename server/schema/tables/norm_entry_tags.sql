-- norm_entry_tags 表结构（MySQL/TiDB 方言，真相源）
-- 分类之外的横切标签。硬删：标签随条目内容重建，没有独立生命周期。

CREATE TABLE `norm_entry_tags` (
  `entry_id` char(32) NOT NULL,
  `tag` varchar(32) NOT NULL,
  PRIMARY KEY (`entry_id`,`tag`),
  KEY `ix_norm_entry_tags_tag` (`tag`),
  CONSTRAINT `fk_norm_entry_tag_entry` FOREIGN KEY (`entry_id`) REFERENCES `norm_entries` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

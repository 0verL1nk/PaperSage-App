-- refresh token 会话链：一次登录 = 一个 family，轮换继承；重放整族吊销。
-- 可空且不回填：滚动发布期间旧二进制不知道新列，NOT NULL 会让其插入失败；
-- 历史行在首次被轮换时惰性补上 family。
ALTER TABLE `refresh_tokens`
  ADD COLUMN `family_id` char(32) DEFAULT NULL,
  ADD COLUMN `replaced_by` varchar(64) DEFAULT NULL,
  ADD KEY `ix_refresh_tokens_family_id` (`family_id`);

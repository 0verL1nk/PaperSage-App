## ADDED Requirements

### Requirement: Test environment isolated from production
E2E 套件 SHALL 在与生产 / 本地开发完全隔离的环境中运行,避免污染或被污染。

#### Scenario: Local .env not loaded in CI
- **WHEN** CI workflow 跑 E2E
- **THEN** workflow `env:` 段显式覆盖所有 `app.config.Settings` 字段,`backend/.env` 被忽略

#### Scenario: Seeded user is deterministic
- **WHEN** `backend/scripts/seed_e2e_user.py` 跑
- **THEN** 创建用户 `e2euser` 密码 `e2epass1`(满足 `^(?=.*[A-Za-z])(?=.*\d).{8,}$`),id 固定

#### Scenario: Seeded project is deterministic
- **WHEN** seed 脚本跑
- **THEN** 创建项目 `default-project` id `11111111-1111-1111-1111-111111111111`,3 个空 latex files

#### Scenario: Idempotent seed
- **WHEN** seed 脚本跑第二次(同 DB)
- **THEN** 不报错,已存在的 user / project 跳过

#### Scenario: MinIO bucket pre-created
- **WHEN** MinIO init 容器跑
- **THEN** `papersage` bucket 存在,access key `ci-access` / secret `ci-secret` 配 `S3_*` env

#### Scenario: Per-job SQLite cleanup
- **WHEN** job 结束
- **THEN** `e2e.db` 不上传 artifact,本地 `rm -f` 清理(避免与下次 run 串数据)

### Requirement: Specs can run in any order or in parallel
E2E 套件 SHALL 设计为可任意顺序或并行跑(2 worker),不依赖执行顺序。

#### Scenario: Specs don't share mutable global state
- **WHEN** 任意 spec 单独跑
- **THEN** 通过(用 `test.beforeEach` 设独立 fixture,不用其它 spec 创建的资源)

#### Scenario: Spec uses unique project name to avoid clash
- **WHEN** spec 创建项目
- **THEN** 项目名带 spec 名前缀(如 `e2e-project-lifecycle`)+ UUID 后缀,避免并行冲突

#### Scenario: Upload spec uses ephemeral folder
- **WHEN** upload spec 创建 folder / doc
- **THEN** folder 名带 `Date.now()` 后缀,job 结束不清理(下次 job 是新 SQLite,无影响)

# PaperSage Go 后端工程最佳实践

> 2026-09 调研结论，用于指导 `server/`（Go 后端）的搭建与移植。结构决策已同步至 `openspec/changes/2026-09-05-migrate-backend-to-go/design.md`（D10）。
>
> 调研来源：[Organizing a Go module（Go 官方）](https://go.dev/doc/modules/layout)、[golang-standards/project-layout](https://github.com/golang-standards/project-layout)（社区模板，非官方）、[Eleven Tips for Structuring Your Go Projects（Alex Edwards）](https://www.alexedwards.net/blog/11-tips-for-structuring-your-go-projects)、[The one-and-only Go project layout（Applied Go）](https://learn.appliedgo.net/blog/go-project-layout)、[A Practical Guide to Structuring a Go Backend](https://dev.to/niraj_maharjan/a-practical-guide-to-structuring-a-go-backend-1jb2)、[Go Project Structure 2026](https://reintech.io/blog/go-project-structure-2026-clean-architecture-best-practices)。

## 1. 本项目采用的布局

```
server/
├── cmd/
│   ├── papersage/        # 主程序（唯一职责：装配依赖并启动）
│   └── seed/             # E2E 数据种子（替代 Python seed 脚本）
├── internal/             # 全部实现代码（编译器强制禁止外部 import）
│   ├── config/           # env 配置（变量名与 Python 版 1:1）
│   ├── httpapi/          # chi 装配、信封 {code,data,info}、错误映射、中间件
│   ├── server/           # HTTP server 生命周期、优雅停机、SPA/embed 静态服务
│   ├── auth/             # 认证域：handler + service + JWT/bcrypt
│   ├── projects/         # 项目/分组/文件夹/回收站域
│   ├── documents/        # 文档上传域
│   ├── latex/            # LaTeX 域：latex_files + CLSI 客户端 + 编译队列
│   ├── settings/         # 用户设置域
│   ├── store/            # DB 句柄、GORM 模型（集中存放）、迁移执行
│   ├── cache/            # Redis / 内存双实现
│   ├── storage/          # S3 客户端
│   └── email/            # Resend / SMTP
├── migrations/           # golang-migrate SQL 文件（embed）
├── web/                  # 前端构建产物（embed，构建时由 frontend/dist 填充）
├── go.mod / go.sum
└── Makefile（构建联动，见根 Makefile）
```

## 2. 布局规则与依据

| 决策 | 依据 |
|---|---|
| 不设 `pkg/` | Go 官方文档通篇未提及 `pkg/`；server 项目「一般不需要导出包」。社区模板里的 `pkg/` 是最常被 cargo-cult 的目录。真要共享，官方建议拆独立模块而非导出包 |
| 实现全进 `internal/` | 官方："keep packages in internal as much as possible"。编译器强制隔离，可自由重构不破坏外部 |
| 命令进 `cmd/<name>/`，main 只做装配 | 官方 server 项目模式。`main.go` 里出现业务逻辑即违规（依赖注入/组装链条：config → store → service → handler → router） |
| `migrations/`、`web/` 在模块内但不进 `internal/` | 非纯 Go 资产（SQL、静态文件），embed 需要它们在模块目录树内 |
| 从平铺起步，不预设企业级分层 | 共识：Go 没有官方目录标准；过度分层（hexagonal/clean 一开始就上 ports/adapters）对小项目是反模式 |

## 3. 包设计原则

- **按域分包，不按技术类型分包**：`auth/` 包含该域的 handler、service、DTO，而不是全局 `handlers/` + `services/` + `models/` 各一大包。域内文件命名 `handler.go` / `service.go` / `dto.go`
- **例外——GORM 模型集中在 `store/`**：跨域外键关系（user→projects→folders/documents→compile_jobs）若按域拆模型极易产生 import 环；集中放置与现有 SQLAlchemy `models/` 一一对应，移植成本最低
- **依赖方向**：`cmd → server → httpapi → 域包 → store/cache/storage`。域包之间**禁止互相 import**（共享走 store 或接口注入）；`store` 不得 import 任何域包
- **接口定义在消费方**：需要 mock 的仓储/外部服务接口写在域包内（如 `auth` 定义 `UserStore` 接口），`store` 包提供实现。这是 Go 打破依赖环的标准手法
- **包名**：全小写、单词、无下划线（`httpapi` 而非 `http_api`）；避免 `utils`/`common`/`helpers` 这类无意义包名

## 3.5 不造轮子原则（用户约定，2026-09-05）

**做事前先找现成轮子**；已写代码定期审查是否有成熟库可替代。只有确实没有轮子时才手写，且须注明理由。

**选库清单**（每项对应一个「 concern 」，禁止手搓替代）：

| 关注点 | 选用的库 | 备注 |
|---|---|---|
| Web 框架 | `gin-gonic/gin` v1.10.x | 最新版要求 Go≥1.25，钉住 1.23 兼容版 |
| CORS | `gin-contrib/cors` | |
| 配置 | `caarlos0/env/v11` + `joho/godotenv` | |
| ORM | `gorm.io/gorm` + `driver/postgres` | |
| SQLite（测试/E2E） | `glebarez/sqlite` | 纯 Go 免 CGO |
| 迁移 | `golang-migrate/v4` v4.18.x | 最新版要求 Go≥1.24，钉住 |
| Redis | `redis/go-redis/v9` | URL 解析用官方 `ParseURL` |
| 内存缓存兜底 | `patrickmn/go-cache` | 不手搓 TTL map |
| JWT | `golang-jwt/jwt/v5` | |
| 口令哈希 | `golang.org/x/crypto/bcrypt` | |
| UUID | `google/uuid` | |
| S3 | `aws-sdk-go-v2/s3` | 官方 SDK |
| 邮件 | `resend/resend-go` / `wneessen/go-mail` | 不手搓 SMTP |
| 测试断言 | `stretchr/testify` | |
| Redis 测试替身 | `alicebob/miniredis/v2` | 纯 Go 内嵌 Redis |
| 日志 | `log/slog` | 标准库 |

**经审查保留的自研**（确无现成轮子，均在此登记理由）：
- `httpapi` 信封/错误映射 — 应用专属 API 契约，无通用库
- panic 恢复中间件 — gin 自带 `Recovery()` 输出纯文本 500，不满足信封契约
- `store.UUID` 双方言适配（PG 原生 uuid / SQLite 32 位无横线 hex）— 复刻 Python TypeDecorator 的存储形态，无现成库；走 GORM 官方扩展点（`GormDBDataType` + `Valuer/Scanner`）
- `dto.JSONTime` — 复刻 Python `datetime.isoformat()` 的 JSON 形态（微秒固定 6 位/省略、`+00:00` 偏移），无通用库
- 迁移 stamp 的两条 SQL — golang-migrate 官方文档推荐的 stamping 方式即手写 INSERT `schema_migrations`

## 4. 代码风格

- `gofmt` 不可协商；import 按三组分段（标准库 / 外部依赖 / 本模块 internal），空行分隔
- 错误是值：逐处 `if err != nil` 显式处理，禁止 `_` 吞错；域内错误用 sentinel（`var ErrNotFound = errors.New(...)`）或类型化错误 + `errors.Is/As`，在 `httpapi` 层统一映射为信封——对应 Python 的 AppException 体系
- handler 不 panic；意外 panic 由中间件兜底并返回与 Python `generic_exception_handler` 逐字段一致的信封
- `context.Context` 永远是第一个参数；DB/HTTP 调用全部感知 ctx 取消
- 接收器命名一两个字母（`(s *Service)`），不用 `this`/`self`
- 不用 `init()` 做实质初始化；显式 `New*()` 构造函数注入依赖
- 日志用 `log/slog`（结构化、stdlib）；JSON handler 在生产、text 在开发

## 5. 测试

- 测试文件与被测代码同目录（`auth/service_test.go`），**不设**顶层 `tests/` 目录——这与 pytest 布局相反，是 Go 的硬惯例
- 表驱动测试（`tests := []struct{...}`）为默认形态
- HTTP 层用 `net/http/httptest` 起 in-process 服务测路由 + 信封形状（等价移植时，pytest 断言的 JSON 形状逐字段搬过来）
- 环境变量用 `t.Setenv`（自动并行安全 + 自动还原）
- 断言库用 `testify`（require/assert）——事实标准，等价移植的断言量级下省掉大量 if/t.Error 模板
- 外部依赖：DB 用 SQLite（`glebarez/sqlite` 纯 Go 免 CGO）；CLSI 用 httptest 假服务；S3 用 MinIO 容器
- 单测内不放集成依赖（本地 `go test ./...` 必须零环境可跑）；需要容器的测试用 build tag 或 `testing.Short()` 跳过

## 6. 工具链与构建

- 本地门槛：`gofmt -l .`（输出必须为空）+ `go vet ./...` + `go test ./...`
- CI 额外跑 `golangci-lint run`（默认 linter 集：errcheck/govet/staticcheck/unused/ineffassign，先不写自定义配置，避免版本漂移）
- `go mod tidy` 提交前必跑；依赖最小化（当前仅 chi / cors / env / godotiny / testify / golangci 相关）
- 二进制产物进 `server/bin/`（已 gitignore），禁止在模块根目录留构建产物
- 交叉编译天然支持（CGO_ENABLED=0），Windows/Linux 双平台产物由 Makefile/Dockerfile 出

## 7. 从 Python 后端迁移的映射表

| Python（backend/） | Go（server/） |
|---|---|
| `api/routes/*.py` 路由函数 | 对应域包 `handler.go` |
| `services/*` | 对应域包 `service.go` |
| `models/*.py`（SQLAlchemy） | `internal/store/model*.go`（GORM，集中） |
| `schemas/*.py`（Pydantic DTO） | 域包 `dto.go`（struct + json tag；校验在 handler 层手写或用 tag） |
| `core/`（db/security/token/cache/email） | `store/` + `auth/` + `cache/` + `email/` |
| `app/exceptions.py` 信封/错误 | `internal/httpapi/errors.go` |
| `app/config.py` | `internal/config/config.go` |
| `alembic/` | `migrations/`（golang-migrate SQL） |
| `tests/conftest.py` fixtures | 各域 `_test.go` 内 helper + `testutil` helper 包 |
| pytest 参数化 | 表驱动测试 |
| `main.py`/`app/main.py` 双入口 | `cmd/papersage/main.go` 单入口 |

## 8. 设计模式应用对照

> 显式列出本项目使用的设计模式及其落点，供代码 review 对照与论文写作引用。Go 的模式实现特点是「用接口与组合替代继承」，比 Java/Python 的类继承形态更轻。

| 模式 | 落点 | 说明 |
|---|---|---|
| **策略模式（Strategy）** | `internal/cache.Store`（MemoryStore / RedisStore） | 同一缓存接口的两个可互换实现，按 REDIS_URL 选择 |
| **工厂模式（Factory）** | `cache.New(redisURL)`、`store.Open(cfg)`、`email` 的 provider 工厂 | 依配置返回具体实现，调用方只依赖接口 |
| **中间件/装饰器（Middleware/Decorator）** | `httpapi` 的 recoverMiddleware、CORS、后续 auth 中间件 | chi 的 `Use` 链，每层包装 http.Handler 增强横切行为 |
| **模板方法（Template Method）** | `store.BaseModel.BeforeCreate` 钩子 | 公共主键生成逻辑，各模型经由嵌入复用（组合实现而非继承） |
| **仓储模式（Repository）** | `internal/store`（GORM 模型 + 各域查询方法） | 数据访问收敛到 store，域 service 通过接口消费 |
| **依赖注入（DI，构造函数注入）** | 各域 `New(deps...)` 构造函数、`cmd/main.go` 装配 | 显式注入，无全局单例（Python 版 `cache_store` 模块级单例在 Go 侧改为注入） |
| **接口定义在消费方** | 各域包内定义所需仓储/服务接口，store 实现之 | Go 打破依赖环的标准手法，也是 mock 测试点 |
| **值对象（Value Object）** | `store.UUID`、`dto.JSONTime` | 封装跨方言/跨格式的序列化语义 |
| **门面（Facade）** | `httpapi.WriteSuccess/WriteError` | 统一信封出口，屏蔽序列化细节 |
| **观察者/回调（后续）** | LaTeX 编译队列的状态回调（任务组 7） | goroutine + channel 实现的轻量事件通知 |

**反面对照**（论文可讨论的技术债）：Python 版的模块级单例（`cache_store`、`settings` 全局）与 `get_settings()` lru_cache 是**单例模式**的隐式用法，跨测试隔离困难；Go 侧改为显式注入是刻意的改进点。

## 9. 反模式清单（Review 时对照）

- [ ] `main.go` 里出现业务逻辑
- [ ] 出现 `pkg/`、`utils/`、`common/`、`helpers/` 包
- [ ] 域包之间互相 import（必然编译报错，出现即设计错了）
- [ ] `store` import 域包（方向反了）
- [ ] handler 直接摸 `*gorm.DB`（应走域 service → 接口）
- [ ] 吞错（`_ = err`）或字符串比较错误（应 `errors.Is/As`）
- [ ] `init()` 里做 IO / 起连接
- [ ] 全局可变状态（单例 map 无锁并发读写）
- [ ] 测试放 `tests/` 顶层目录、或断言实现细节而非行为
- [ ] 结构体 JSON tag 与 Python 序列化字段名不一致（等价移植破坏点）

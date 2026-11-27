# 前端设计系统与组件复用规范

> 适用范围：`frontend/src`。本文件是 HELE-55 P1 + P2 的落地说明；页面迁移按 P3–P6 分批进行。
> 根目录 `DESIGN.md` 是早期 Apple 风格参考稿，与当前实现不一致，仅作历史参考。

## 1. 分层与依赖方向

沿用仓库既有三层，不新增 `components/common/`：

| 层 | 目录 | 放什么 | 约束 |
|---|---|---|---|
| 原语 | `components/ui/` | shadcn / Radix 生成物与最小扩展 | 不含业务语义，不依赖 `shared` / `domain` |
| 通用组合 | `components/shared/` | 页面骨架、四态、状态徽标、分段筛选 | 纯展示，不发请求；只依赖 `ui` 与 `lib` |
| 业务组件 | `components/domain/` | ProjectCard、NormLibraryPanel 等 | 可依赖 `shared` / `ui`，反向禁止 |

新组件统一从 `@/components/shared` barrel 导入。抽组件前先检索 `ui/` 与 `shared/`；同一 UI 结构出现第三次再抽（Rule of Three）。

## 2. Token 单一真源（`frontend/src/index.css`）

- **圆角总闸**：`--radius: 0.5rem`。派生：`sm 4.8px`、`md 6.4px`、`lg 8px`、`xl 11.2px`、`2xl 14.4px`、`3xl 17.6px`、`4xl 20.8px`。页面不要再写 `rounded-[28px]` 一类 arbitrary 值；大容器用 `rounded-xl`，超大面板用 `rounded-2xl`。
- **字体链只在 `@theme` 定义一次**：`--font-sans` 含 `PingFang SC / Hiragino Sans GB / Noto Sans SC / Microsoft YaHei` 回退；`--font-mono` 含 `SF Mono / Menlo / Consolas / Noto Sans Mono CJK SC`。`@theme inline` 不得再次覆盖 `--font-sans`（Tailwind v4 同名变量会覆盖，导致中文回退失效）。不引入中文 webfont（体积会击穿 size-limit）。
- **页面底色由 token 控制**：`body` 不再写死渐变；亮/暗主题都走 `--background`。业务组件禁止 `bg-white`（纸张/PDF 预览除外）。
- **语义色**：`--success` / `--warning` / `--info` / `--destructive` 均已提供亮/暗值，并在 `@theme inline` 映射为 `text-*` / `bg-*` / `border-*`。状态展示只用这些语义色，不再散用 `amber / emerald / green / sky`。文件类型图标色板是文档化的例外，不属于状态色。

### 排印阶梯

| 角色 | class | 字号 / 行高 / 字重 | 用途 |
|---|---|---|---|
| Display | `text-display` | 40 / 1.1 / 600 | 认证页主标题 |
| H1 | `text-h1` | 30 / 1.2 / 600 | 页面标题（PageHeader） |
| H2 | `text-h2` | 24 / 1.3 / 600 | 区块标题 |
| H3 | `text-h3` | 18 / 1.4 / 600 | 卡片标题 |
| Body | `text-body` | 15 / 1.7 / 400 | 正文 |
| Body-sm | `text-body-sm` | 13 / 1.6 / 400 | 辅助说明、表格 |
| Caption | `text-caption` | 12 / 1.5 / 500 | 元信息、徽标 |
| Mono | `font-mono text-body-sm` | 13 / 1.6 | 日志、代码、路径 |

## 3. 通用组件目录（`components/shared/`）

| 组件 | 用途 | 关键 Props |
|---|---|---|
| `PageContainer` | 页面留白与最大宽度 | `size: narrow \| default \| wide` |
| `PageHeader` | 页面标题区 | `title`、`description`、`eyebrow`、`icon`、`actions`、`back`（已渲染的返回链接） |
| `SectionCard` | 区块卡片 | `title`、`description`、`actions`、`size`、`contentClassName` |
| `PanelToolbar` | 面板工具条 | `title`、`subtitle`、`start`、`end`、`sticky`、`bordered` |
| `StateBlock` | 四态基座 | `title`、`description`、`icon`、`action`、`compact` |
| `EmptyState` | 空态 | 同上 |
| `ErrorState` | 错误态（含重试） | `details`、`onRetry`、`retryLabel`（默认 i18n `common.retry`） |
| `LoadingState` | 加载态 | `variant: skeleton \| spinner`、`rows` |
| `StatusBadge` | 语义状态徽标 | `tone: neutral \| info \| success \| warning \| danger`、`dot` |
| `SegmentedControl` | 分段筛选（Radix ToggleGroup） | `items`、`value`、`onValueChange`、`ariaLabel`、`size` |

使用规则：

- `SectionCard` 内不再嵌 `SectionCard`；内部分区用 `Separator` 或浅色 `div`。
- 视图切换（准备页 chat/brief、工作区阶段）继续用 Radix `Tabs`，不与 `SegmentedControl` 混用语义。
- 组件只收 props，不内置中文文案；按钮文案走 i18n（`common.confirm / cancel / processing / retry`）。
- 删除/危险操作使用 `ui/confirm-dialog.tsx`，不再使用 `window.confirm`。

## 4. 自检命令

```bash
cd frontend
pnpm typecheck && pnpm lint

# shared 层不得出现 arbitrary radius / raw palette
grep -rnE "rounded-\[|-(amber|emerald|green|sky|slate|gray)-[0-9]" \
  src/components/shared src/views
# 预期：仅剩文件类型色板与白名单项（rounded-[inherit]、min(var(--radius-md),…)）

NODE_OPTIONS=--max-old-space-size=2560 pnpm exec vitest run \
  src/components/shared/design-system.test.tsx --maxWorkers=1
```

全量回归交给 GitHub CI（`frontend.yml` + `e2e.yml`）。视觉验收按 10 页 × 3 视口 × 亮/暗矩阵进行。

## 5. 后续批次

- P3：`MainLayout` / `WorkspaceLayout` / 项目列表 / 回收站 / 规范库 / 设置 / 管理后台。
- P4：准备页 / 检查页 / AI 面板 / 规范库面板。
- P5：写作页 / 资料库（等 HELE-47 合入后）。
- P6：认证页 + 全站暗色与 320px 收口。

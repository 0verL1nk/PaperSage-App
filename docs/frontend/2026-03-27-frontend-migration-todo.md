# PaperSage Frontend Migration Todo

关联计划文档: [2026-03-27-frontend-migration-plan.md](/home/ling/GraduateProject/docs/2026-03-27-frontend-migration-plan.md)

## 状态约定

- `[x]` 已完成
- `[ ]` 未开始
- `[-]` 进行中
- `[!]` 有风险或需要额外确认

## Phase 0: 已完成基础脚手架

- [x] 在 `/home/ling/GraduateProject/frontend` 初始化 React + TypeScript Vite 项目
- [x] 使用命令 `pnpm create vite@latest . -- --template react-ts`
- [x] 确认前端包管理器固定为 `pnpm`

## Phase 1: 基础工程化与设计系统

### 1.0 视觉基线

- [x] 先定义整体视觉方向，不等功能做完再补样式
- [x] 确定基础色板和语义色 token
- [x] 确定字体层级和字号比例
- [x] 确定圆角、阴影、边框、间距规则
- [x] 确定桌面端和移动端的布局基线
- [x] 确定 loading / empty / error 的统一视觉样式

### 1.1 依赖与脚本

- [x] 检查并整理 `frontend/package.json`
- [x] 安装运行时依赖
  - `@tanstack/react-query`
  - `@tanstack/react-router`
  - `zustand`
  - `react-hook-form`
  - `zod`
  - `@hookform/resolvers`
  - `lucide-react`
  - `axios`
  - `md5`
  - `pdfjs-dist`
- [x] 安装开发依赖
  - `tailwindcss`
  - `postcss`
  - `autoprefixer`
  - `@types/node`
  - `typescript` 相关缺失项
- [x] 在 `package.json` 中补齐脚本
  - `dev`
  - `build`
  - `preview`
  - `typecheck`
  - `lint`

### 1.2 Tailwind 与 shadcn/ui

- [x] 初始化 Tailwind 配置
- [x] 配置全局样式入口
- [x] 初始化 `shadcn/ui`
- [x] 选择并确认 Radix-based preset
- [x] 生成 `components.json`
- [x] 添加首批基础组件
  - `button`
  - `card`
  - `dialog`
  - `form`
  - `input`
  - `textarea`
  - `tabs`
  - `table`
  - `sheet`
  - `dropdown-menu`
  - `tooltip`
  - `alert`
  - `badge`
  - `skeleton`
  - `separator`
  - `scroll-area`
  - `sonner`

### 1.3 目录骨架

- [x] 建立 `src/app`
- [x] 建立 `src/routes`
- [x] 建立 `src/shared`
- [x] 建立 `src/features`
- [x] 建立按 domain 划分的子目录
  - `auth`
  - `projects`
  - `trash`
  - `workspace`
  - `uploading`
  - `coding`

### 1.4 本阶段验证

- [x] 执行 `pnpm install`
- [x] 执行 `pnpm build`
- [x] 执行 `pnpm typecheck`
- [x] 确认 Tailwind 和 shadcn 样式已生效

## Phase 2: 应用基础设施

### 2.1 环境与配置

- [x] 建立 `src/shared/config/env.ts`
- [x] 约定 `VITE_API_BASE_URL`
- [x] 补充 `frontend/.env.example`
- [x] 明确开发环境与生产环境 API 访问方式

### 2.2 API 客户端

- [x] 建立统一 API 客户端 `src/shared/api/client.ts`
- [x] 定义通用响应结构 `{ code, data, info }`
- [x] 实现错误归一化
- [x] 实现 token 注入
- [x] 实现 `multipart/form-data` 提交支持
- [x] 实现 blob 下载支持
- [x] 使用 `Bearer <accessToken>` 注入 `Authorization` 头

### 2.3 全局状态

- [x] 建立 `session-store`
- [x] 建立 `ui-store`
- [x] 保存和读取 `accessToken` / `refreshToken`
- [x] 实现全局 loading 计数器
- [x] 实现登录态恢复逻辑

### 2.4 Provider 层

- [x] 创建 QueryClient
- [x] 创建 RouterProvider
- [x] 接入 toast provider
- [x] 接入全局 loading overlay

### 2.5 本阶段验证

- [x] 基础 provider 可以启动
- [x] 不接业务接口时页面可正常渲染
- [x] API client 能被路由和 hooks 正常引用

## Phase 3: 路由与页面壳层

### 3.1 根路由

- [x] 创建 `src/routes/__root.tsx`
- [x] 配置全局 layout
- [x] 放置全局 toast / loading / outlet

### 3.2 认证与主路由

- [x] 创建 `/auth`
- [x] 创建 `/main/documents`
- [x] 创建 `/main/trash`
- [x] 创建 `/main/profile`
- [x] 建立鉴权守卫
- [x] 未登录时跳转 `/auth`

### 3.3 项目工作区路由

- [x] 创建 `/project/$projectId`
- [x] 创建 `/project/$projectId/uploading`
- [x] 创建 `/project/$projectId/coding`
- [x] 创建 `/project/$projectId/classifying`
- [x] 创建 `/project/$projectId/logic-chain`
- [x] 创建 `/project/$projectId/generating`

### 3.4 壳层组件

- [x] 实现主站 layout
  - 头部
  - 左侧导航
  - 内容区域
- [x] 主站 layout 做到可直接作为正式产品 UI 使用，不保留脚手架默认观感
- [x] 实现项目工作区 layout
  - 返回按钮
  - 首页按钮
  - 顶部 tab 导航
- [x] 给 workspace 导航和内容区建立明确的视觉层级

### 3.5 本阶段验证

- [x] 每个路由都能打开
- [x] 未登录保护生效
- [x] layout 在桌面端正常显示

## Phase 4: Auth 模块

### 4.1 数据与校验

- [x] 定义 login schema
- [x] 定义 register schema
- [x] 校验用户名非空
- [x] 校验密码复杂度
- [x] 校验注册时确认密码

### 4.2 接口与状态

- [x] 实现登录 mutation
- [x] 实现注册 mutation
- [x] 登录前密码做 SHA-256 (原计划 MD5，已升级更安全的方案)
- [x] 登录成功后落 `accessToken` / `refreshToken`
- [x] 登录成功后跳转 documents / projects

### 4.3 页面与交互

- [x] 实现登录/注册切换 UI
- [x] 显示提交错误
- [x] 显示提交中状态
- [x] 保留“记住我”策略的迁移决定
- [x] 不保留旧版“记住我”
- [x] 删掉旧行为并在文档记录

### 4.4 本阶段验证

- [x] 未登录打开主路由会被重定向
- [x] 登录成功后进入 `/app/projects`
- [x] 注册成功后行为符合预期

## Phase 5: 项目列表与回收站

### 5.1 项目列表接口

- [x] 实现项目列表 query
- [x] 实现创建项目 mutation
- [x] 实现创建分组 mutation
- [x] 实现删除项目 mutation

### 5.2 Documents 页面

- [x] 实现分组 tabs
- [x] 实现项目搜索
- [x] 实现项目卡片列表
- [x] 设计项目卡片的视觉样式
  - 封面/占位区
  - 标题信息层级
  - hover 态
  - 操作菜单态
- [x] 实现新建项目弹窗
- [x] 实现新建分组弹窗
- [x] 实现项目删除入口
- [x] 点击项目跳转工作区

### 5.3 Trash 页面

- [x] 实现回收站列表 query
- [x] 实现恢复 mutation
- [x] 实现永久删除 mutation
- [x] 实现空状态

### 5.4 本阶段验证

- [x] 创建项目后列表自动刷新
- [x] 删除项目后回收站可见
- [x] 恢复项目后 documents 可见
- [x] 搜索只影响当前分组列表

## Phase 6: Uploading 模块

### 6.1 文件夹与文件列表

- [ ] 实现文件夹列表 query
- [ ] 实现文件列表 query
- [ ] 明确 folder tree 的数据结构映射
- [ ] 支持按当前 folder 切换文件列表

### 6.2 上传能力

- [ ] 实现上传 mutation
- [ ] 约束支持文件类型
  - `pdf`
  - `doc`
  - `docx`
  - `txt`
- [ ] 实现上传进度展示
- [ ] 上传后刷新列表

### 6.3 批量操作

- [ ] 实现文件多选
- [ ] 实现全选/取消全选
- [ ] 实现移动文件
- [ ] 实现备注编辑
- [ ] 实现删除文件

### 6.4 预览面板

- [ ] 实现右侧预览容器
- [ ] 支持 PDF 预览
- [ ] 支持 TXT 预览
- [ ] 处理加载状态
- [ ] 处理预览失败状态

### 6.5 上传页 UI

- [ ] 实现 toolbar
- [ ] 实现 file table
- [ ] 实现 folder tree
- [ ] 实现 remark dialog
- [ ] 实现 move dialog
- [ ] 优化文件表格的密度、状态色和可读性
- [ ] 优化预览区与列表区的比例关系

### 6.6 本阶段验证

- [ ] 上传后文件出现在列表
- [ ] 选择文件后批量按钮状态正确
- [ ] 备注、移动、删除均能成功刷新
- [ ] 点击文件标题能打开预览

## Phase 7: Coding 模块

### 7.1 左侧文件来源

- [ ] 确认 coding 页面文件选择来源
- [ ] 复用 uploading 侧的文件查询能力，避免重复接口逻辑
- [ ] 明确“打开文件到 tab”动作来源

### 7.2 Tab 状态

- [ ] 建立 opened files store
- [ ] 保存 active file
- [ ] 支持关闭 tab
- [ ] 支持切换 tab
- [ ] 明确是否保留 localStorage 恢复逻辑

### 7.3 PDF 与高亮

- [ ] 实现文件 URL 获取
- [ ] 实现 blob PDF 加载
- [ ] 实现 highlights 获取
- [ ] 解析 highlights JSON
- [ ] 高亮数据传递到右侧面板

### 7.4 Coding 页面 UI

- [ ] 实现顶部 file tabs
- [ ] 实现 PDF viewer 主区域
- [ ] 实现 highlights panel
- [ ] 实现 loading/progress 展示
- [ ] 文件切换时清理旧高亮
- [ ] 优化阅读界面的沉浸感和视觉安静度
- [ ] 保证 PDF 区、tab 区、右侧高亮区层次明确

### 7.5 本阶段验证

- [ ] 打开文件后 tab 正确新增
- [ ] 切换 tab 后 PDF 正确切换
- [ ] 右侧高亮与当前文件一致
- [ ] 关闭当前 tab 后能切到邻近文件

## Phase 8: 占位页面与收尾路由

### 8.1 占位页

- [ ] 为 `classifying` 添加占位页
- [ ] 为 `logic-chain` 添加占位页
- [ ] 为 `generating` 添加占位页
- [ ] 页面文案明确“后续迁移/未实现”

### 8.2 路由一致性

- [ ] 确保 Vue 中真实存在的路由在 React 中都有对应页
- [ ] 明确不迁移的 `labels` 和 `more`
- [ ] 在文档中记录未迁移项

### 8.3 本阶段验证

- [ ] 项目工作区所有 tab 都可访问
- [ ] 占位页面不会导致白屏或路由异常

## Phase 9: 质量保障

### 9.1 代码质量

- [ ] 配置 ESLint
- [ ] 配置 TypeScript 检查
- [ ] 统一 import alias
- [ ] 清理无用脚手架代码

### 9.2 测试

- [ ] 引入 Vitest
- [ ] 引入 React Testing Library
- [ ] 为 Auth 表单加最小测试
- [ ] 为路由鉴权加最小测试
- [ ] 为至少一个 query 驱动页面加最小测试

### 9.3 文档

- [ ] 编写 `frontend/README.md`
- [ ] 说明启动命令
- [ ] 说明环境变量
- [ ] 说明依赖后端接口
- [ ] 说明当前已迁移/未迁移范围
- [ ] 说明视觉规范是否已沉淀为 theme/token 约定

### 9.4 本阶段验证

- [ ] `pnpm install`
- [ ] `pnpm build`
- [ ] `pnpm typecheck`
- [ ] `pnpm lint`
- [ ] 测试命令可运行

## Phase 10: 切换准备

### 10.1 迁移验收

- [ ] Auth 可用
- [ ] Documents 可用
- [ ] Trash 可用
- [ ] Uploading 可用
- [ ] Coding 可用
- [ ] 占位页齐全

### 10.2 风险清单复核

- [ ] 客户端 MD5 登录兼容性已验证
- [ ] `Bearer` token 注入格式已验证
- [ ] PDF.js 资源加载路径已验证
- [ ] 高亮接口返回格式已验证

### 10.3 交付前整理

- [ ] 清理调试日志
- [ ] 清理无用组件
- [ ] 核对 `frontend` 最终目录结构
- [ ] 核对文档与实际实现一致

## 建议执行顺序

1. 先做 Phase 1-3，把壳和基础设施立起来。
2. 再做 Phase 4-5，先把认证和项目列表打通。
3. 然后做 Phase 6-7，优先上传，再做 coding。
4. 最后做 Phase 8-10，补占位页、测试、文档和切换准备。

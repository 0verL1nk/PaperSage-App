## Why

PaperSage React 迁移已完成 Phase 1-2 的基础设施搭建，但还未建立有效的路由系统和页面壳层。用户无法通过 URL 访问不同功能模块，也无法获得一致的布局体验。必须建立路由与页面壳层，为后续 Auth 和业务功能提供入口和导航框架。

## What Changes

- **根路由配置**：创建 `src/routes/__root.tsx`，配置全局 layout，放置全局 toast / loading / outlet
- **认证与主路由**：创建 `/auth`、`/app/documents`、`/app/trash`、`/app/profile`，建立鉴权守卫，未登录时跳转 `/auth`
- **项目工作区路由**：创建 `/project/$projectId` 及子路由（uploading、coding、classifying、logic-chain、generating）
- **主站布局组件**：实现主站 layout（头部、左侧导航、内容区域），支持桌面端显示
- **工作区布局组件**：实现项目工作区 layout（返回按钮、首页按钮、顶部 tab 导航）

## Capabilities

### New Capabilities
- **routing-system**: 定义应用路由结构、根路由配置、路由守卫逻辑
- **main-layout**: 主站通用布局，包含顶部导航、侧边栏、内容区
- **workspace-layout**: 项目工作区布局，包含顶部 tab 导航和内容区

### Modified Capabilities
- (无)

## Impact

- **frontend/src/routes/**：新增路由文件结构
- **frontend/src/features/**：各模块的页面入口
- 依赖 `@tanstack/react-router` 已安装
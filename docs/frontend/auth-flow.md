# 前端鉴权与 Token 管理

## 概述

本文档描述 PaperSage 前端的鉴权流程和 Token 管理机制。

## Token 类型

| Token | 用途 | 存储方式 |
|-------|------|----------|
| `accessToken` | API 请求认证 | localStorage (`papersage_access_token`) |
| `refreshToken` | 刷新 accessToken | localStorage (`papersage_refresh_token`) |

## 核心模块

### 1. Session Store (`src/shared/stores/session-store.ts`)

负责 Token 的持久化和状态管理：

- `setTokens(tokens)` - 保存 Token 到 localStorage
- `clearTokens()` - 清除 Token
- `restoreSession()` - 页面加载时恢复 Token
- `getAccessToken()` - 获取当前 accessToken
- `getRefreshToken()` - 获取当前 refreshToken
- `hasSessionToken()` - 判断是否已登录

### 2. API Client (`src/shared/api/client.ts`)

负责 Token 的自动注入和刷新：

#### 请求拦截器
- 自动在请求头添加 `Authorization: Bearer <accessToken>`
- 显示/隐藏全局 Loading

#### 响应拦截器 (Token 刷新逻辑)
```
1. 收到 401 响应
2. 检查是否正在刷新 Token：
   - 是：等待刷新完成，订阅重试
   - 否：开始刷新流程
3. 调用刷新接口 `/api/v1/auth/refresh`
4. 刷新成功：
   - 保存新 Token
   - 通知所有等待的请求重试
   - 重试原始请求
5. 刷新失败：
   - 清除本地 Token
   - 跳转到登录页
```

## API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/account/login` | POST | 登录（返回 accessToken + refreshToken） |
| `/api/account/register` | POST | 注册 |
| `/api/v1/auth/refresh` | POST | 刷新 Token |

## 登录/注册流程

1. 用户提交用户名密码
2. 密码使用 MD5 哈希
3. 使用 `multipart/form-data` 提交到 `/api/account/login`
4. 响应包含 accessToken 和 refreshToken
5. 调用 `setSessionTokens()` 保存到 localStorage

## 路由守卫

使用 `@tanstack/react-router` 的 `beforeLoad` 钩子：

- 未登录访问受保护路由 → 重定向到 `/auth`
- 已登录用户访问 `/auth` → 重定向到 `/app/documents`
- 首页 `/` → 已登录跳 `/app/documents`，未登录跳 `/auth`

## 测试

```bash
# 运行所有测试
pnpm test

# 测试文件
src/features/auth/api.test.ts          # 登录/注册 API 测试
src/features/auth/components/auth-page.test.tsx  # 登录页组件测试
src/shared/api/token-refresh.test.ts   # Token 刷新模块测试
```

## 常见问题

### Q: 为什么 Token 刷新需要单独的 axios 实例？
避免刷新请求本身触发拦截器，造成无限循环。

### Q: 如何禁用 Token 刷新？
在请求配置中设置 `skipAuthRefresh: true`（仅限特殊场景）。

### Q: 刷新失败会怎样？
- 清除 localStorage 中的 Token
- 跳转到 `/auth` 登录页
- 用户需要重新登录
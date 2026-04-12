# API 接口规格

## 1. 接口概览

### 1.1 Base URL

```
开发环境: http://localhost:8000/api
生产环境: https://api.<your-domain>/api
```

### 1.2 认证方式

除公开接口外，所有请求需要在 Header 中携带 Access Token：

```
Authorization: Bearer <access_token>
```

### 1.3 响应格式

所有响应使用统一信封格式：

```typescript
interface ApiResponse<T> {
  code: number;    // 状态码 (200=成功)
  data: T | null;  // 响应数据
  info: string;    // 附加信息（错误时为错误消息）
}
```

### 1.4 前后端约定

#### URL 必须使用 trailing slash

后端 FastAPI 路由定义使用 trailing slash（如 `@router.get("/")`），前端 API 调用**必须使用 trailing slash**：

```typescript
// ✅ 正确
const url = '/api/projects/'

// ❌ 错误 - 会触发 307 重定向
const url = '/api/projects'
```

原因：FastAPI 的 `redirect_slashes` 默认为 `True`，访问 `/api/projects` 会重定向到 `/api/projects/`。

#### 前端类型定义

前端 `unwrapApiEnvelope` 已自动解包 `data` 字段，直接返回 `T`。因此 TanStack Query hooks 应直接使用数据类型：

```typescript
// ✅ 正确 - unwrapApiEnvelope 返回 T[]
const data = await get<Project[]>('/api/projects/')
return data ?? []

// ❌ 错误 - 重复解包导致 undefined
const data = await get<{ data: Project[] }>('/api/projects/')
return data?.data ?? [] // data 已经是数组，data.data 是 undefined
```

### 1.5 状态码说明

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 201 | 创建成功 |
| 400 | 请求参数错误 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 422 | 业务逻辑错误 |
| 500 | 服务器内部错误 |

---

## 2. 认证接口

### 2.1 用户注册

```
POST /api/auth/register
Content-Type: multipart/form-data
```

**请求参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名，3-20字符 |
| password | string | 是 | 密码（MD5加密） |
| email | string | 否 | 邮箱 |

**响应示例：**

```json
{
  "code": 200,
  "data": {
    "id": "user_xxx",
    "username": "zhangsan",
    "email": "zhangsan@example.com"
  },
  "info": ""
}
```

### 2.2 用户登录

```
POST /api/auth/login
Content-Type: multipart/form-data
```

**请求参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码（MD5加密） |
| captcha | string | 否 | 验证码 |

**响应示例：**

```json
{
  "code": 200,
  "data": {
    "accessToken": "eyJhbGciOiJIUzI1NiIs...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": "user_xxx",
      "username": "zhangsan",
      "email": "zhangsan@example.com"
    }
  },
  "info": ""
}
```

### 2.3 刷新 Token

```
POST /api/auth/refresh
Content-Type: application/json
```

**请求参数：**

```json
{
  "refreshToken": "eyJhbGciOiJIUzI1NiIs..."
}
```

### 2.4 退出登录

```
POST /api/auth/logout
Authorization: Bearer <access_token>
```

**响应：**

```json
{
  "code": 200,
  "data": null,
  "info": ""
}
```

---

## 3. 项目管理接口

### 3.1 获取项目列表

```
GET /api/projects
Authorization: Bearer <access_token>
```

**Query 参数：**

| 参数 | 类型 | 说明 |
|------|------|------|
| group_id | string | 按分组筛选（可选） |
| page | number | 页码，默认1 |
| limit | number | 每页数量，默认20 |

**响应示例：**

```json
{
  "code": 200,
  "data": {
    "items": [
      {
        "id": "proj_xxx",
        "name": "我的毕业设计",
        "description": "毕业论文写作项目",
        "groupId": "group_xxx",
        "status": "active",
        "createdAt": "2026-04-01T10:00:00Z",
        "updatedAt": "2026-04-07T10:00:00Z"
      }
    ],
    "total": 10,
    "page": 1,
    "limit": 20
  },
  "info": ""
}
```

### 3.2 创建项目

```
POST /api/projects
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数：**

```json
{
  "name": "新项目",
  "groupId": "group_xxx",
  "description": "项目描述"
}
```

### 3.3 更新项目

```
PUT /api/projects/:projectId
Authorization: Bearer <access_token>
Content-Type: application/json
```

**请求参数：**

```json
{
  "name": "新名称",
  "description": "新描述"
}
```

### 3.4 删除项目

```
DELETE /api/projects/:projectId
Authorization: Bearer <access_token>
```

### 3.5 获取项目分组列表

```
GET /api/project-groups
Authorization: Bearer <access_token>
```

### 3.6 创建项目分组

```
POST /api/project-groups
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "name": "分组名称"
}
```

---

## 4. 文档管理接口

### 4.1 获取文件夹树

```
GET /api/projects/:projectId/folders
Authorization: Bearer <access_token>
```

**响应示例：**

```json
{
  "code": 200,
  "data": [
    {
      "id": "folder_xxx",
      "name": "参考文献",
      "parentId": null,
      "children": [
        {
          "id": "folder_yyy",
          "name": "PDF文献",
          "parentId": "folder_xxx"
        }
      ]
    }
  ],
  "info": ""
}
```

### 4.2 创建文件夹

```
POST /api/folders
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "projectId": "proj_xxx",
  "parentId": "folder_xxx",
  "name": "新文件夹"
}
```

### 4.2.1 删除文件夹

```
DELETE /api/folders/:folderId
Authorization: Bearer <access_token>
```

### 4.3 获取文档列表

```
GET /api/folders/:folderId/documents
Authorization: Bearer <access_token>
```

或获取项目根目录文档：

```
GET /api/projects/:projectId/documents
Authorization: Bearer <access_token>
```

### 4.4 上传文档

```
POST /api/documents/upload
Authorization: Bearer <access_token>
Content-Type: multipart/form-data
```

| 参数 | 类型 | 说明 |
|------|------|------|
| file | File | 文件 |
| projectId | string | 项目ID |
| folderId | string | 文件夹ID（可选） |
| remark | string | 备注（可选） |

**响应：**

```json
{
  "code": 200,
  "data": {
    "id": "doc_xxx",
    "name": "paper.pdf",
    "type": "pdf",
    "size": 1024000,
    "status": "processing"
  },
  "info": ""
}
```

### 4.5 获取文档预览URL

```
GET /api/documents/:documentId/preview
Authorization: Bearer <access_token>
```

**响应：**

```json
{
  "code": 200,
  "data": {
    "url": "https://minio.example.com/bucket/xxx.pdf?token=..."
  },
  "info": ""
}
```

### 4.6 删除文档

```
DELETE /api/documents/:documentId
Authorization: Bearer <access_token>
```

### 4.7 移动文档

```
PUT /api/documents/:documentId/move
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "folderId": "folder_xxx"
}
```

### 4.8 编辑文档备注

```
PUT /api/documents/:documentId/remark
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "remark": "新的备注内容"
}
```

---

## 5. 高亮与批注接口

### 5.1 获取高亮列表

```
GET /api/documents/:documentId/highlights
Authorization: Bearer <access_token>
```

**响应：**

```json
{
  "code": 200,
  "data": [
    {
      "id": "hl_xxx",
      "page": 1,
      "content": "这是高亮的文本内容",
      "color": "yellow",
      "note": "这是批注",
      "position": {"x": 100, "y": 200, "width": 300, "height": 20},
      "createdAt": "2026-04-01T10:00:00Z"
    }
  ],
  "info": ""
}
```

### 5.2 创建高亮

```
POST /api/highlights
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "documentId": "doc_xxx",
  "page": 1,
  "content": "高亮文本",
  "color": "yellow",
  "note": "批注内容",
  "position": {"x": 100, "y": 200, "width": 300, "height": 20}
}
```

### 5.3 更新高亮

```
PUT /api/highlights/:highlightId
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "color": "green",
  "note": "更新后的批注"
}
```

### 5.4 删除高亮

```
DELETE /api/highlights/:highlightId
Authorization: Bearer <access_token>
```

---

## 6. 回收站接口

### 6.1 获取回收站列表

```
GET /api/trash
Authorization: Bearer <access_token>
```

**响应：**

```json
{
  "code": 200,
  "data": {
    "projects": [...],
    "documents": [...]
  },
  "info": ""
}
```

### 6.2 恢复项目

```
POST /api/trash/projects/:projectId/restore
Authorization: Bearer <access_token>
```

### 6.3 恢复文档

```
POST /api/trash/documents/:documentId/restore
Authorization: Bearer <access_token>
```

### 6.4 永久删除项目

```
DELETE /api/trash/projects/:projectId
Authorization: Bearer <access_token>
```

### 6.5 永久删除文档

```
DELETE /api/trash/documents/:documentId
Authorization: Bearer <access_token>
```

---

## 7. 个人中心接口

### 7.1 获取用户信息

```
GET /api/user/profile
Authorization: Bearer <access_token>
```

### 7.2 修改密码

```
PUT /api/user/password
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "oldPassword": "old_password_md5",
  "newPassword": "new_password_md5"
}
```

---

## 8. AI 接口（待实现）

### 8.1 生成摘要

```
POST /api/ai/summarize
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "documentId": "doc_xxx"
}
```

### 8.2 生成大纲

```
POST /api/ai/outline
Authorization: Bearer <access_token>
Content-Type: application/json
```

```json
{
  "projectId": "proj_xxx",
  "topic": "论文主题"
}
```

---

## 9. 错误响应示例

### 9.1 400 错误

```json
{
  "code": 400,
  "data": null,
  "info": "项目名称不能为空"
}
```

### 9.2 401 错误

```json
{
  "code": 401,
  "data": null,
  "info": "登录已过期，请重新登录"
}
```

### 9.3 404 错误

```json
{
  "code": 404,
  "data": null,
  "info": "项目不存在"
}
```

### 9.4 500 错误

```json
{
  "code": 500,
  "data": null,
  "info": "服务器内部错误"
}
```

---

*本章节详细描述了 PaperSage 系统的所有 API 接口规格，包括请求参数、响应格式和错误处理。*
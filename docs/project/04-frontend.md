# 前端实现详情

## 1. 前端技术体系

### 1.1 核心依赖

```json
{
  "dependencies": {
    "react": "^19.2.4",
    "@tanstack/react-query": "^5.95.2",
    "@tanstack/react-router": "^1.168.7",
    "zustand": "^5.0.12",
    "react-hook-form": "^7.72.0",
    "zod": "^4.3.6",
    "axios": "^1.13.6",
    "pdfjs-dist": "^5.5.207",
    "lucide-react": "^1.7.0",
    "sonner": "^2.0.7"
  },
  "devDependencies": {
    "typescript": "~5.9.3",
    "vite": "^8.0.1",
    "tailwindcss": "^4.2.2",
    "shadcn": "^4.1.1",
    "vitest": "^4.1.2",
    "eslint": "^9.39.4"
  }
}
```

### 1.2 构建配置

**Vite 配置** (`vite.config.ts`):

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
})
```

**TypeScript 配置** (`tsconfig.json`):

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedSideEffectImports": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  }
}
```

## 2. 路由实现

### 2.1 路由定义

使用 TanStack Router 实现类型安全的路由：

```typescript
// frontend/src/app/router.tsx

import { createRouter, createRootRouteWithContext, createRoute, redirect } from '@tanstack/react-router'
import { hasSessionToken } from '@/shared/stores/session-store'

// 认证守卫
function requireAuth() {
  if (!hasSessionToken()) {
    throw redirect({ to: '/auth', search: { redirect: window.location.pathname } })
  }
}

// 根路由
const rootRoute = createRootRouteWithContext<AppRouterContext>()({
  component: () => <Outlet />,
})

// 首页 - 重定向
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    if (hasSessionToken()) {
      throw redirect({ to: '/app/documents' })
    } else {
      throw redirect({ to: '/auth' })
    }
  },
})

// 登录页
const authRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth',
  beforeLoad: () => {
    if (hasSessionToken()) {
      throw redirect({ to: '/app/documents' })
    }
  },
  component: AuthPage,
})

// 应用布局（需认证）
const appLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app-layout',
  component: () => <MainLayout><Outlet /></MainLayout>,
  beforeLoad: () => { requireAuth() },
})

// 文档列表
const documentsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/documents',
  component: DocumentsPage,
})

// 路由树组装
const routeTree = rootRoute.addChildren([
  indexRoute,
  authRoute,
  appLayoutRoute.addChildren([documentsRoute, trashRoute, profileRoute]),
  projectLayoutRoute.addChildren([...projectRoutes]),
])

export const router = createRouter({ routeTree, context: { queryClient: undefined! } })
```

### 2.2 路由清单

| 路径 | 组件 | 认证 | 说明 |
|------|------|------|------|
| `/` | Index | 自动重定向 | 首页 |
| `/auth` | AuthPage | 否 | 登录/注册 |
| `/app/documents` | DocumentsPage | 是 | 项目列表 |
| `/app/trash` | TrashPage | 是 | 回收站 |
| `/app/profile` | ProfilePage | 是 | 个人中心 |
| `/project/$projectId` | ProjectRoot | 是 | 项目概览 |
| `/project/$projectId/uploading` | UploadingPage | 是 | 上传模块 |
| `/project/$projectId/coding` | CodingPage | 是 | 编码模块 |
| `/project/$projectId/classifying` | ClassifyingPage | 是 | 分类模块 |
| `/project/$projectId/logic-chain` | LogicChainPage | 是 | 逻辑链 |
| `/project/$projectId/generating` | GeneratingPage | 是 | 内容生成 |

## 3. 状态管理

### 3.1 服务端状态 (TanStack Query)

```typescript
// frontend/src/shared/api/client.ts

import axios from 'axios'
import { getAccessToken } from '@/shared/stores'

const apiClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 30000,
})

// 请求拦截 - 添加 Token
apiClient.interceptors.request.use((config) => {
  const token = getAccessToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截 - 统一错误处理
apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      clearSessionTokens()
      window.location.href = '/auth'
    }
    return Promise.reject(error)
  }
)

export default apiClient
```

```typescript
// frontend/src/features/projects/hooks.ts

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '@/shared/api/client'
import type { Project, ProjectGroup } from './schema'

// 获取项目列表
export function useProjects(groupId?: string) {
  return useQuery({
    queryKey: ['projects', groupId],
    queryFn: () => apiClient.get<{ data: Project[] }>('/projects', { params: { groupId } }),
    select: (res) => res.data,
  })
}

// 创建项目
export function useCreateProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { name: string; groupId?: string }) =>
      apiClient.post<{ data: Project }>('/projects', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}
```

### 3.2 客户端状态 (Zustand)

```typescript
// frontend/src/shared/stores/session-store.ts

import { create } from 'zustand'

const ACCESS_TOKEN_KEY = 'papersage_access_token'
const REFRESH_TOKEN_KEY = 'papersage_refresh_token'

interface SessionState {
  accessToken: string
  refreshToken: string
  restored: boolean
  setTokens: (tokens: { accessToken: string; refreshToken: string }) => void
  clearTokens: () => void
  restoreSession: () => { accessToken: string; refreshToken: string }
}

export const useSessionStore = create<SessionState>((set, get) => ({
  accessToken: '',
  refreshToken: '',
  restored: false,

  setTokens: (tokens) => {
    localStorage.setItem(ACCESS_TOKEN_KEY, tokens.accessToken)
    localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refreshToken)
    set({ ...tokens, restored: true })
  },

  clearTokens: () => {
    localStorage.removeItem(ACCESS_TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
    set({ accessToken: '', refreshToken: '', restored: true })
  },

  restoreSession: () => {
    const tokens = {
      accessToken: localStorage.getItem(ACCESS_TOKEN_KEY) || '',
      refreshToken: localStorage.getItem(REFRESH_TOKEN_KEY) || '',
    }
    set({ ...tokens, restored: true })
    return tokens
  },
}))

export const hasSessionToken = () => Boolean(useSessionStore.getState().accessToken)
```

```typescript
// frontend/src/features/uploading/store.ts

import { create } from 'zustand'

interface UploadState {
  selectedFolderId: string | null
  selectedFileIds: string[]
  isUploading: boolean
  uploadProgress: Map<string, number>

  setSelectedFolder: (id: string | null) => void
  toggleFileSelection: (id: string) => void
  selectAllFiles: (ids: string[]) => void
  clearSelection: () => void
  setUploading: (uploading: boolean) => void
  setProgress: (fileId: string, progress: number) => void
}

export const useUploadStore = create<UploadState>((set) => ({
  selectedFolderId: null,
  selectedFileIds: [],
  isUploading: false,
  uploadProgress: new Map(),

  setSelectedFolder: (id) => set({ selectedFolderId: id }),
  toggleFileSelection: (id) => set((state) => ({
    selectedFileIds: state.selectedFileIds.includes(id)
      ? state.selectedFileIds.filter((fid) => fid !== id)
      : [...state.selectedFileIds, id],
  })),
  selectAllFiles: (ids) => set({ selectedFileIds: ids }),
  clearSelection: () => set({ selectedFileIds: [] }),
  setUploading: (uploading) => set({ isUploading: uploading }),
  setProgress: (fileId, progress) => set((state) => {
    const newMap = new Map(state.uploadProgress)
    newMap.set(fileId, progress)
    return { uploadProgress: newMap }
  }),
}))
```

## 4. 表单处理

### 4.1 表单 Schema 定义

使用 Zod 定义表单验证规则：

```typescript
// frontend/src/features/auth/schema.ts

import { z } from 'zod'

export const loginSchema = z.object({
  username: z.string().min(3, '用户名至少3个字符').max(20, '用户名最多20个字符'),
  password: z.string().min(6, '密码至少6个字符'),
})

export const registerSchema = z.object({
  username: z.string()
    .min(3, '用户名至少3个字符')
    .max(20, '用户名最多20个字符')
    .regex(/^[a-zA-Z0-9_]+$/, '用户名只能包含字母、数字和下划线'),
  password: z.string()
    .min(6, '密码至少6个字符')
    .max(50, '密码最多50个字符'),
  confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
  message: '两次输入的密码不一致',
  path: ['confirmPassword'],
})

export type LoginInput = z.infer<typeof loginSchema>
export type RegisterInput = z.infer<typeof registerSchema>
```

### 4.2 表单组件

```typescript
// frontend/src/features/auth/components/auth-page.tsx

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { loginSchema, registerSchema, type LoginInput, type RegisterInput } from '../schema'
import { useLogin, useRegister } from '../hooks'
import md5 from 'md5'

export function AuthPage() {
  const [isRegister, setIsRegister] = useState(false)
  const login = useLogin()
  const register = useRegister()

  const form = useForm<LoginInput | RegisterInput>({
    resolver: zodResolver(isRegister ? registerSchema : loginSchema),
    defaultValues: { username: '', password: '', confirmPassword: '' },
  })

  const onSubmit = (data: LoginInput | RegisterInput) => {
    // MD5 加密密码（兼容旧版 API）
    const payload = {
      ...data,
      password: md5(data.password),
      ...(isRegister && data.confirmPassword ? { confirmPassword: md5(data.confirmPassword) } : {}),
    }

    if (isRegister) {
      register.mutate(payload as RegisterInput & { password: string })
    } else {
      login.mutate(payload as LoginInput & { password: string })
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{isRegister ? '注册' : '登录'}</CardTitle>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>用户名</FormLabel>
                    <FormControl>
                      <Input {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>密码</FormLabel>
                    <FormControl>
                      <Input type="password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {isRegister && (
                <FormField
                  control={form.control}
                  name="confirmPassword"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>确认密码</FormLabel>
                      <FormControl>
                        <Input type="password" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              <Button type="submit" className="w-full">
                {isRegister ? '注册' : '登录'}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
```

## 5. API 客户端

### 5.1 统一响应格式

后端返回统一信封格式：

```typescript
// frontend/src/shared/api/contracts.ts

export interface ApiEnvelope<T> {
  code: number      // 200 = 成功
  data: T
  info: string      // 错误信息或附加信息
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  limit: number
}
```

### 5.2 错误处理

```typescript
// frontend/src/shared/api/errors.ts

export class ApiError extends Error {
  constructor(
    public code: number,
    message: string,
    public statusCode?: number
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function handleApiError(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message
  }
  if (axios.isAxiosError(error)) {
    if (error.response) {
      return error.response.data?.info || '请求失败'
    }
    return '网络错误'
  }
  return '未知错误'
}
```

## 6. PDF 渲染

### 6.1 PDF.js 集成

```typescript
// frontend/src/features/coding/components/pdf-viewer.tsx

import { useEffect, useRef, useState } from 'react'
import * as pdfjsLib from 'pdfjs-dist'

// 设置 Worker
pdfjsLib.GlobalWorkerOptions.workerSrc = `https://cdnjs.cloudflare.com/ajax/libs/pdf.js/${pdfjsLib.version}/pdf.worker.min.js`

interface PdfViewerProps {
  url: string
  onLoad?: (numPages: number) => void
}

export function PdfViewer({ url, onLoad }: PdfViewerProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [pdf, setPdf] = useState<pdfjsLib.PDFDocumentProxy | null>(null)
  const [currentPage, setCurrentPage] = useState(1)
  const [scale, setScale] = useState(1.5)

  useEffect(() => {
    pdfjsLib.getDocument(url).promise.then((doc) => {
      setPdf(doc)
      onLoad?.(doc.numPages)
    })
  }, [url])

  useEffect(() => {
    if (!pdf || !canvasRef.current) return

    pdf.getPage(currentPage).then((page) => {
      const canvas = canvasRef.current!
      const context = canvas.getContext('2d')!
      const viewport = page.getViewport({ scale })

      canvas.height = viewport.height
      canvas.width = viewport.width

      page.render({
        canvasContext: context,
        viewport,
      })
    })
  }, [pdf, currentPage, scale])

  return (
    <div className="flex flex-col items-center">
      <canvas ref={canvasRef} className="border" />
      <div className="flex items-center gap-2 mt-4">
        <Button onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}>上一页</Button>
        <span>{currentPage} / {pdf?.numPages || 0}</span>
        <Button onClick={() => setCurrentPage((p) => Math.min(pdf!.numPages, p + 1))}>下一页</Button>
        <Select value={scale.toString()} onValueChange={(v) => setScale(Number(v))}>
          <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="1">100%</SelectItem>
            <SelectItem value="1.5">150%</SelectItem>
            <SelectItem value="2">200%</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
```

## 7. UI 组件

### 7.1 组件库

基于 shadcn/ui 构建的组件：

| 组件 | 用途 |
|------|------|
| Button | 按钮 |
| Input | 输入框 |
| Card | 卡片容器 |
| Dialog | 对话框 |
| DropdownMenu | 下拉菜单 |
| Table | 表格 |
| Tabs | 标签页 |
| Form | 表单 |
| Toast/Sonner | 提示消息 |
| Skeleton | 加载占位 |
| Badge | 标签 |
| Alert | 警告提示 |

### 7.2 布局组件

**主布局 (MainLayout)**:

```typescript
// frontend/src/features/layout/MainLayout.tsx

export function MainLayout({ children }: { children: ReactNode }) {
  const location = useLocation()
  const currentPath = location.pathname

  return (
    <div className="flex min-h-screen flex-col">
      {/* 顶部导航 */}
      <header className="sticky top-0 z-50 flex h-16 items-center border-b bg-background/95 px-6">
        <Link to="/" className="flex items-center gap-2">
          <Home className="size-5" />
          <span className="text-lg font-semibold">PaperSage</span>
        </Link>
      </header>

      <div className="flex flex-1">
        {/* 侧边栏 */}
        <aside className="hidden w-64 flex-col border-r lg:flex">
          <nav className="flex-1 space-y-1 p-4">
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={cn(
                  'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium',
                  currentPath === item.path ? 'bg-accent' : 'text-muted-foreground'
                )}
              >
                <item.icon className="size-4" />
                {item.label}
              </Link>
            ))}
          </nav>
        </aside>

        {/* 主内容区 */}
        <main className="flex-1">{children}</main>
      </div>
    </div>
  )
}
```

## 8. 开发规范

### 8.1 代码组织原则

1. **Feature-First**: 按业务功能组织代码，而非按文件类型
2. **Colocation**: 相关文件放在一起（组件、样式、测试）
3. **Shared 隔离**: 公共代码放在 shared 目录

### 8.2 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 组件 | PascalCase | `AuthPage.tsx` |
| Hooks | camelCase, use 前缀 | `useProjects.ts` |
| 工具函数 | camelCase | `cn.ts` |
| 类型/接口 | PascalCase | `Project` |
| 常量 | UPPER_SNAKE_CASE | `ACCESS_TOKEN_KEY` |

### 8.3 导入路径

使用路径别名 `@/`：

```typescript
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { useProjects } from '@/features/projects/hooks'
import { hasSessionToken } from '@/shared/stores/session-store'
```

---

*本章节详细描述了 PaperSage 前端的实现细节，包括路由、状态管理、表单处理、API 客户端等核心模块。*
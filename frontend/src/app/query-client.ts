import { QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { friendlyErrorMessage } from '@/services/errors'

/**
 * 页面查询用 networkMode: 'always'。TanStack Query 默认的 'online' 会在断网时
 * 直接挂起查询：queryFn 和任何客户端超时都不会启动，页面就永远停在加载态，
 * 用户看不到错误与重试。让查询照常发起，传输层失败或超时后由页面错误态接管。
 */
export function createQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => {
        const description = friendlyErrorMessage(error)
        // 与 requestApi 同策略：同错替换上一条，避免并发查询失败时 toast 堆叠（HELE-22 S6）。
        toast.error('请求失败', { id: `query-error:${description}`, description, duration: 4000 })
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: false,
        networkMode: 'always',
      },
      mutations: {
        retry: false,
      },
    },
  })
}

import { QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

/**
 * 页面查询用 networkMode: 'always'。TanStack Query 默认的 'online' 会在断网时
 * 直接挂起查询：queryFn 和任何客户端超时都不会启动，页面就永远停在加载态，
 * 用户看不到错误与重试。让查询照常发起，传输层失败或超时后由页面错误态接管。
 */
export function createQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => {
        toast.error('请求失败', { description: error.message, duration: 4000 })
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

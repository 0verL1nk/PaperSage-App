import { useQuery } from '@tanstack/react-query'

import { getRetrievalMode } from '@/services/health-api'

/**
 * 当前后端的资料检索能力。
 *
 * null 表示读不到（旧后端 / 网络问题）——界面按“未知”处理，不提示、不报错：
 * 这是说明性信息，读取失败不该把页面变成错误态。
 */
export function useRetrievalMode() {
  const { data } = useQuery({
    queryKey: ['retrieval-mode'],
    queryFn: getRetrievalMode,
    retry: false,
    staleTime: 5 * 60_000,
  })
  return data ?? null
}

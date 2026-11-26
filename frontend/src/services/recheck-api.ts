/** 资料变更后的「待复查」标记 API hooks（服务端产物需要登录；游客不隐式上传）。 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { get, PAGE_QUERY_TIMEOUT_MS, post } from '@/services'
import { GuestCapabilityError } from '@/services/guest-local-workspace'
import { useSessionStore } from '@/store'
import type { RecheckMark, RecheckResolution } from '@/types/research'
import { RECHECK } from './api-routes'

export type RecheckStatus = 'open' | 'resolved'

function requireSignedIn() {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('待复查提醒')
}

export const recheckKeys = {
  list: (projectId: string, status: RecheckStatus) => ['recheck-marks', projectId, status] as const,
}

export function useRecheckMarks(projectId: string | undefined, status: RecheckStatus = 'open') {
  return useQuery({
    queryKey: recheckKeys.list(projectId ?? '', status),
    enabled: Boolean(projectId),
    queryFn: async () => {
      requireSignedIn()
      return (await get<RecheckMark[]>(RECHECK.list(projectId!, status), {
        timeout: PAGE_QUERY_TIMEOUT_MS,
        // 面板自带加载/错误态，不再叠加全局 toast。
        skipGlobalLoading: true,
      })) ?? []
    },
  })
}

export function useResolveRecheckMark(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ markId, resolution }: { markId: string; resolution: RecheckResolution }) => {
      requireSignedIn()
      return post<RecheckMark>(RECHECK.resolve(projectId, markId), { resolution })
    },
    onSuccess: () => {
      // 处理后面板徽标、产物徽标与采用按钮的可用状态都要刷新。
      void qc.invalidateQueries({ queryKey: ['recheck-marks', projectId] })
      void qc.invalidateQueries({ queryKey: ['research-artifacts', projectId] })
    },
  })
}

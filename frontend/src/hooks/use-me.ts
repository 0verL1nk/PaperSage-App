import { useQuery } from '@tanstack/react-query'
import { getJson } from '@/services/client'

export interface Me {
  username: string
  email: string | null
  emailVerified: boolean
  isSuperuser: boolean
}

/** 当前用户信息（/auth/me）；401 时返回 null（未登录） */
export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => getJson<Me>('/api/v1/auth/me'),
    retry: false,
    staleTime: 60_000,
  })
}

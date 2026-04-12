import { useMutation } from '@tanstack/react-query'
import { loginWithPassword, registerWithPassword } from '@/services/auth-api'

export function useLoginMutation() {
  return useMutation({
    mutationFn: loginWithPassword,
  })
}

export function useRegisterMutation() {
  return useMutation({
    mutationFn: registerWithPassword,
  })
}

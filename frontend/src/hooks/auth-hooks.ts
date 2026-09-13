import { useMutation } from '@tanstack/react-query'
import { loginWithPassword, registerWithEmail } from '@/services/auth-api'

export function useLoginMutation() {
  return useMutation({
    mutationFn: loginWithPassword,
  })
}

export function useRegisterMutation() {
  return useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      registerWithEmail(email, password),
  })
}

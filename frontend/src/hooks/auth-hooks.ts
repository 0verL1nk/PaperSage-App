import { useMutation } from '@tanstack/react-query'
import {
  loginWithPassword,
  registerWithEmail,
  sendEmailCode,
  setPassword,
  verifyEmailCode,
} from '@/services/auth-api'

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

export function useSendEmailCodeMutation() {
  return useMutation({
    mutationFn: sendEmailCode,
  })
}

export function useVerifyEmailCodeMutation() {
  return useMutation({
    mutationFn: ({ email, code }: { email: string; code: string }) =>
      verifyEmailCode(email, code),
  })
}

export function useSetPasswordMutation() {
  return useMutation({
    mutationFn: setPassword,
  })
}

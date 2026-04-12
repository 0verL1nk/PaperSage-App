import { z } from 'zod'

export const PASSWORD_PATTERN = /^(?=.*[A-Za-z])(?=.*\d).{8,}$/

const usernameSchema = z.string().trim().min(1, '请输入用户名')
const passwordSchema = z
  .string()
  .min(1, '请输入密码')
  .regex(PASSWORD_PATTERN, '密码必须包含字母和数字，且长度至少为 8 位')

export const loginSchema = z.object({
  username: usernameSchema,
  password: passwordSchema,
})

export const registerSchema = loginSchema
  .extend({
    confirmPassword: z.string().min(1, '请再次输入密码'),
  })
  .refine((value) => value.password === value.confirmPassword, {
    path: ['confirmPassword'],
    message: '两次输入的密码不一致',
  })

export type LoginFormValues = z.infer<typeof loginSchema>
export type RegisterFormValues = z.infer<typeof registerSchema>

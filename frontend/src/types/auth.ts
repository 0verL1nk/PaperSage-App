import { z } from 'zod'

// NIST SP 800-63B Rev4：不设复杂度规则，仅长度门槛
const passwordSchema = z
  .string()
  .min(1, '请输入密码')
  .min(8, '密码至少 8 位')

const accountSchema = z.string().trim().min(1, '请输入用户名或邮箱')

const emailSchema = z
  .string()
  .trim()
  .min(1, '请输入学校邮箱')
  .email('邮箱格式不正确')

export const loginSchema = z.object({
  username: accountSchema,
  password: passwordSchema,
})

export const registerSchema = z
  .object({
    email: emailSchema,
    password: passwordSchema,
    confirmPassword: z.string().min(1, '请再次输入密码'),
  })
  .refine((value) => value.password === value.confirmPassword, {
    path: ['confirmPassword'],
    message: '两次输入的密码不一致',
  })

export const emailCodeLoginSchema = z.object({
  // 验证码登录不限于学校邮箱（域名白名单由后端按部署配置），文案保持中性。
  email: z
    .string()
    .trim()
    .min(1, '请输入邮箱')
    .email('邮箱格式不正确'),
  code: z
    .string()
    .trim()
    .min(1, '请输入验证码')
    .regex(/^\d{6}$/, '请输入 6 位数字验证码'),
})

/** 设置/修改密码表单；是否要求旧密码由页面按 hasPassword 再校验。 */
export function passwordFormSchema() {
  return z
    .object({
      currentPassword: z.string(),
      newPassword: passwordSchema,
      confirmPassword: z.string().min(1, '请再次输入新密码'),
    })
    .refine((value) => value.newPassword === value.confirmPassword, {
      path: ['confirmPassword'],
      message: '两次输入的密码不一致',
    })
}

export type LoginFormValues = z.infer<typeof loginSchema>
export type RegisterFormValues = z.infer<typeof registerSchema>
export type EmailCodeLoginFormValues = z.infer<typeof emailCodeLoginSchema>
export type PasswordFormValues = z.infer<ReturnType<typeof passwordFormSchema>>

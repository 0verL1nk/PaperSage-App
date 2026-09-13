import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight, Eye, EyeOff, MailCheck, ShieldCheck } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { SmoothHeight } from '@/components/shared/smooth-height'
import { useQuery } from '@tanstack/react-query'
import { useLoginMutation, useRegisterMutation } from '@/hooks/auth-hooks'
import { getEmailDomains, resendVerification } from '@/services/auth-api'
import { cn } from '@/lib/utils'
import {
  loginSchema,
  registerSchema,
  type LoginFormValues,
  type RegisterFormValues,
} from '@/types/auth'
import { setSessionTokens, useSessionStore } from '@/store'

type AuthMode = 'login' | 'register'

function getErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }
  return '操作失败，请稍后重试。'
}

/** 后端 403 "Please verify your email" → 登录被验证门槛拦截 */
function isVerifyRequired(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error.message.includes('verify your email') ||
      error.message.includes('完成邮箱验证'))
  )
}

/** 重发验证邮件按钮：60s 实时倒计时（服务端冷却同窗口） */
function ResendButton({ email, className }: { email: string; className?: string }) {
  const [left, setLeft] = useState(0)
  useEffect(() => {
    if (left <= 0) return
    const t = setInterval(() => setLeft((n) => n - 1), 1000)
    return () => clearInterval(t)
  }, [left])

  const send = () => {
    resendVerification(email)
      .then(() => {
        toast.success('已重新发送', { description: '若该邮箱已注册且未验证，验证邮件已重新发送。' })
        setLeft(60)
      })
      .catch(() => toast.error('发送失败，请稍后再试'))
  }

  return (
    <button
      type="button"
      disabled={left > 0}
      className={cn('text-primary hover:underline disabled:opacity-60 disabled:no-underline', className)}
      onClick={send}
    >
      {left > 0 ? `重新发送（${left}s）` : '没收到？重新发送'}
    </button>
  )
}

/** 注册页展示当前允许的邮箱域（公开端点；空 = 不限制时隐藏） */
function AllowedDomainsHint() {
  const { data } = useQuery({
    queryKey: ['auth-email-domains'],
    queryFn: getEmailDomains,
    staleTime: 5 * 60_000,
  })
  if (!data || data.length === 0) return null
  return (
    <p className="text-[11px] text-muted-foreground/70 ml-1">
      目前支持：{data.join('、')}
    </p>
  )
}

export function AuthPage() {
  const navigate = useNavigate()
  const accessToken = useSessionStore((state) => state.accessToken)
  const loginMutation = useLoginMutation()
  const registerMutation = useRegisterMutation()
  const [mode, setMode] = useState<AuthMode>('login')
  const [authError, setAuthError] = useState('')
  const [needsVerify, setNeedsVerify] = useState(false)
  // 注册成功后待验证邮箱（常驻提醒，登录成功即清除）
  const [pendingVerifyEmail, setPendingVerifyEmail] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  useEffect(() => {
    if (accessToken) {
      navigate({ to: '/app/projects' })
    }
  }, [accessToken, navigate])

  const loginForm = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: '', password: '' },
  })

  const registerForm = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: { email: '', password: '', confirmPassword: '' },
  })

  const handleLoginSubmit = loginForm.handleSubmit(async (values) => {
    setAuthError('')
    setNeedsVerify(false)
    try {
      const tokens = await loginMutation.mutateAsync(values)
      setPendingVerifyEmail('')
      setSessionTokens(tokens)
      toast.success('登录成功', { description: '欢迎回来，PaperSage 会话已建立。' })
      await navigate({ to: '/app/projects' })
    } catch (error) {
      setAuthError(getErrorMessage(error))
      setNeedsVerify(isVerifyRequired(error))
    }
  })

  const handleRegisterSubmit = registerForm.handleSubmit(async (values) => {
    setAuthError('')
    try {
      await registerMutation.mutateAsync({
        email: values.email,
        password: values.password,
      })
      setPendingVerifyEmail(values.email)
      toast.success('注册成功', { description: '验证邮件已发送，请查收并完成激活。' })
      setMode('login')
    } catch (error) {
      setAuthError(getErrorMessage(error))
    }
  })

  return (
    <div className="relative flex h-screen w-screen items-center justify-center overflow-hidden bg-background font-sans">
      {/* 银灰色系 Mesh 背景 (适配明暗主题) */}
      <div className="absolute inset-0 z-0 overflow-hidden pointer-events-none">
        {/* 光斑 1: 银白色/浅灰色 */}
        <div className="absolute top-[-10%] left-[-10%] h-[80%] w-[80%] rounded-full bg-zinc-400/10 dark:bg-zinc-600/10 blur-[120px] animate-mesh" />
        {/* 光斑 2: 暖色调银色/琥珀色提示 */}
        <div className="absolute bottom-[10%] right-[-5%] h-[70%] w-[70%] rounded-full bg-amber-200/5 dark:bg-amber-500/5 blur-[100px] animate-mesh" style={{ animationDelay: '-5s' }} />
        {/* 光斑 3: 中灰色 */}
        <div className="absolute top-[20%] right-[10%] h-[60%] w-[60%] rounded-full bg-zinc-300/10 dark:bg-zinc-800/10 blur-[80px] animate-mesh" style={{ animationDelay: '-10s' }} />
        
        {/* 微弱噪点层提升质感 */}
        <div className="absolute inset-0 bg-noise opacity-[0.03] dark:opacity-[0.05] mix-blend-overlay" />
      </div>

      <div className="relative z-10 w-full max-w-[420px] px-6 animate-in fade-in zoom-in-95 duration-700">
        {/* Branding */}
        <div className="mb-10 flex flex-col items-center text-center">
          <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-foreground shadow-xl">
            <span className="text-3xl font-bold italic tracking-tighter text-background">S</span>
          </div>
          <h1 className="text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
            PaperSage
          </h1>
          <p className="mt-2 text-[15px] font-medium text-muted-foreground">
            智能化科研文献管理工作台
          </p>
        </div>

        {/* 统一风格的 Auth Card */}
        <Card className="apple-popover overflow-hidden rounded-[2rem]">
          <CardContent className="p-0">
            <Tabs value={mode} onValueChange={(v) => {
              setAuthError('')
              setNeedsVerify(false)
              setMode(v as AuthMode)
            }} className="w-full">
              <div className="px-8 pt-8">
                <TabsList className="grid w-full grid-cols-2 rounded-xl bg-secondary/50 p-1">
                  <TabsTrigger value="login" className="rounded-lg text-sm font-semibold transition-all duration-200">
                    账号登录
                  </TabsTrigger>
                  <TabsTrigger value="register" className="rounded-lg text-sm font-semibold transition-all duration-200">
                    注册新账号
                  </TabsTrigger>
                </TabsList>
              </div>

              <SmoothHeight className="px-8 pb-6 pt-8" data-testid="auth-panels">
                <TabsContent value="login" className="mt-0 space-y-6 outline-none animate-in fade-in slide-in-from-left-2 duration-300">
                  {pendingVerifyEmail && (
                    <div className="flex items-start gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
                      <MailCheck className="mt-0.5 size-4 flex-none text-primary/70" />
                      <div className="flex-1 text-[13px] leading-relaxed text-foreground/80">
                        验证邮件已发送至 <span className="font-semibold">{pendingVerifyEmail}</span>，
                        请查收并点击邮件中的链接激活账号后再登录（24 小时内有效，注意垃圾箱）。
                        <ResendButton email={pendingVerifyEmail} className="ml-1" />
                      </div>
                    </div>
                  )}
                  {needsVerify && loginForm.getValues('username') && (
                    <div className="text-xs">
                      <ResendButton email={loginForm.getValues('username')} className="font-medium" />
                    </div>
                  )}
                  {authError && (
                    <Alert variant="destructive" className="border-none bg-destructive/10 text-destructive rounded-xl py-3">
                      <ShieldCheck className="mr-2 h-4 w-4" />
                      <AlertDescription className="text-xs font-bold leading-none">{authError}</AlertDescription>
                    </Alert>
                  )}
                  <form onSubmit={handleLoginSubmit} className="space-y-5">
                    <div className="space-y-2.5">
                      <Label htmlFor="username" className="ml-1 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">用户名</Label>
                      <Input
                        id="username"
                        {...loginForm.register('username')}
                        placeholder="请输入您的账号"
                        className="h-12 bg-secondary/30 border-none px-4 rounded-xl focus-visible:ring-1 focus-visible:ring-primary/50"
                      />
                      {loginForm.formState.errors.username && (
                        <p className="text-xs text-destructive ml-1">{loginForm.formState.errors.username.message}</p>
                      )}
                    </div>
                    <div className="space-y-2.5">
                      <div className="flex items-center justify-between px-1">
                        <Label htmlFor="password" className="text-[11px] font-bold uppercase tracking-widest text-muted-foreground">密码</Label>
                        <button type="button" className="text-[11px] font-bold text-primary hover:opacity-80 transition-opacity">忘记密码？</button>
                      </div>
                      <div className="relative">
                        <Input
                          id="password"
                          type={showPassword ? 'text' : 'password'}
                          {...loginForm.register('password')}
                          placeholder="••••••••"
                          className="h-12 bg-secondary/30 border-none px-4 pr-12 rounded-xl focus-visible:ring-1 focus-visible:ring-primary/50"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-4 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        </button>
                      </div>
                      {loginForm.formState.errors.password && (
                        <p className="text-xs text-destructive ml-1">{loginForm.formState.errors.password.message}</p>
                      )}
                    </div>
                    <Button 
                      type="submit" 
                      variant="outline"
                      disabled={loginMutation.isPending}
                      className="flex h-12 w-full font-bold text-[15px] mt-4 rounded-full"
                    >
                      {loginMutation.isPending ? '正在连接...' : '立即登录'}
                      {!loginMutation.isPending && <ArrowRight className="ml-2 h-4 w-4" />}
                    </Button>
                  </form>
                </TabsContent>

                <TabsContent value="register" className="mt-0 space-y-6 outline-none animate-in fade-in slide-in-from-right-2 duration-300">
                  <form onSubmit={handleRegisterSubmit} className="space-y-5">
                    <div className="space-y-2.5">
                      <Label htmlFor="reg-email" className="ml-1 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">学校邮箱</Label>
                      <Input
                        id="reg-email"
                        type="email"
                        {...registerForm.register('email')}
                        placeholder="your-id@hit.edu.cn"
                        className="h-12 bg-secondary/30 border-none px-4 rounded-xl"
                      />
                      {registerForm.formState.errors.email && (
                        <p className="text-xs text-destructive ml-1">{registerForm.formState.errors.email.message}</p>
                      )}
                      <AllowedDomainsHint />
                    </div>
                    <div className="space-y-2.5">
                      <Label htmlFor="reg-password" className="ml-1 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">设置密码</Label>
                      <Input
                        id="reg-password"
                        type="password"
                        {...registerForm.register('password')}
                        placeholder="至少 8 位，包含字母与数字"
                        className="h-12 bg-secondary/30 border-none px-4 rounded-xl"
                      />
                      {registerForm.formState.errors.password && (
                        <p className="text-xs text-destructive ml-1">{registerForm.formState.errors.password.message}</p>
                      )}
                    </div>
                    <div className="space-y-2.5">
                      <Label htmlFor="reg-confirm" className="ml-1 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">确认密码</Label>
                      <Input
                        id="reg-confirm"
                        type="password"
                        {...registerForm.register('confirmPassword')}
                        placeholder="请再次输入密码"
                        className="h-12 bg-secondary/30 border-none px-4 rounded-xl"
                      />
                      {registerForm.formState.errors.confirmPassword && (
                        <p className="text-xs text-destructive ml-1">{registerForm.formState.errors.confirmPassword.message}</p>
                      )}
                    </div>
                    <Button
                      type="submit"
                      variant="outline"
                      disabled={registerMutation.isPending}
                      className="flex h-12 w-full font-bold text-[15px] mt-4 rounded-full"
                    >
                      {registerMutation.isPending ? '正在创建...' : '创建我的账号'}
                    </Button>
                  </form>
                </TabsContent>
              </SmoothHeight>
            </Tabs>
          </CardContent>
          <div className="border-t border-border/20 bg-secondary/20 px-8 py-4 text-center">
            <p className="text-[11px] font-medium text-muted-foreground">
              继续操作即代表您同意我们的 <a href="#" className="text-foreground font-semibold hover:underline transition-all">服务条款</a> 和 <a href="#" className="text-foreground font-semibold hover:underline transition-all">隐私政策</a>
            </p>
          </div>
        </Card>
      </div>
    </div>
  )
}

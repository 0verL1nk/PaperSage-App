import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  ChevronRight,
  Eye,
  EyeOff,
  HardDrive,
  LoaderCircle,
  LogIn,
  MailCheck,
  ShieldCheck,
  UserPlus,
} from 'lucide-react'
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
import { initializeGuestWorkspace } from '@/services/guest-local-workspace'
import { ApiTargetToggle } from '@/components/shared/api-target-toggle'
import { isDesktop } from '@/lib/desktop'
import { DesktopAppMenu, WindowControls } from '@/components/desktop/desktop-titlebar'
import { getApiTarget } from '@/lib/env'
import { cn } from '@/lib/utils'
import {
  loginSchema,
  registerSchema,
  type LoginFormValues,
  type RegisterFormValues,
} from '@/types/auth'
import { activateLocalGuestSession, setSessionTokens, useSessionStore } from '@/store'

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
    <p className="ml-1 text-[11px] text-muted-foreground/70">
      目前支持：{data.join('、')}
    </p>
  )
}

/** 表单控件共用的紧凑样式，保持登录 / 注册两栏观感一致。 */
const fieldInputClass =
  'h-10 rounded-[10px] border-input/70 bg-muted/40 px-3 text-[13.5px] transition-colors placeholder:text-muted-foreground/60 focus-visible:border-ring focus-visible:bg-background focus-visible:ring-1 focus-visible:ring-ring/50'
const fieldLabelClass = 'text-[12.5px] font-medium text-foreground/80'

export function AuthPage() {
  const navigate = useNavigate()
  const accessToken = useSessionStore((state) => state.accessToken)
  const guestImportPending = useSessionStore((state) => state.guestImportPending)
  const loginMutation = useLoginMutation()
  const registerMutation = useRegisterMutation()
  const [mode, setMode] = useState<AuthMode>('login')
  const [authError, setAuthError] = useState('')
  const [needsVerify, setNeedsVerify] = useState(false)
  // 注册成功后待验证邮箱（常驻提醒，登录成功即清除）
  const [pendingVerifyEmail, setPendingVerifyEmail] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [guestStarting, setGuestStarting] = useState(false)
  const [guestError, setGuestError] = useState('')

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
      toast.success('请求已处理', { description: '如果该邮箱需要验证，请查收邮件；未收到时可重试注册以重新发送。' })
      setMode('login')
    } catch (error) {
      setAuthError(getErrorMessage(error))
    }
  })

  const handleGuestStart = async () => {
    if (guestStarting) return
    setGuestError('')
    setGuestStarting(true)
    try {
      await initializeGuestWorkspace()
      activateLocalGuestSession()
      await navigate({ to: '/app/projects' })
    } catch (cause) {
      setGuestError(cause instanceof Error ? cause.message : '无法开启本机游客工作区，请检查浏览器存储设置后重试。')
    } finally {
      setGuestStarting(false)
    }
  }

  return (
    <div className="relative flex h-full min-h-0 w-full overflow-hidden bg-background font-sans">
      {/* 银灰色系 Mesh 背景 (适配明暗主题) */}
      <div aria-hidden="true" className="pointer-events-none absolute inset-0 z-0 overflow-hidden">
        <div data-testid="auth-background-orb" className="auth-aurora auth-mesh absolute -left-[22%] -top-[25%] h-[78%] w-[86%] rounded-full blur-[96px]" />
        <div className="auth-aurora-warm auth-mesh-reverse absolute -bottom-[20%] -right-[20%] h-[72%] w-[78%] rounded-full blur-[100px]" />
        <div className="auth-aurora-cool auth-mesh absolute right-[4%] top-[12%] h-[52%] w-[54%] rounded-full blur-[90px] [animation-delay:-11s]" />

        <div className="auth-grid absolute inset-0" />

        {/* 微弱噪点层提升质感 */}
        <div className="absolute inset-0 bg-noise opacity-[0.03] mix-blend-overlay dark:opacity-[0.05]" />
      </div>

      <div className="relative z-10 grid h-full w-full grid-cols-1 overflow-y-auto overscroll-contain lg:grid-cols-[minmax(0,1fr)_minmax(0,520px)] lg:overflow-hidden">
        {/* 品牌栏：≥1024px 显示，垂直居中 */}
        <aside className="hidden lg:flex lg:items-center lg:justify-center lg:px-14 xl:px-20">
          <div className="flex w-full max-w-[340px] flex-col items-center text-center">
            <div className="relative w-fit">
              <div
                aria-hidden="true"
                className="absolute -inset-5 rounded-[28px] bg-accent/15 blur-2xl"
              />
              <img
                src="/app-icon.png"
                alt=""
                className="relative size-[56px] rounded-[18px] shadow-[0_18px_40px_-18px_rgba(15,23,42,0.45)] ring-1 ring-black/5 dark:ring-white/10"
              />
            </div>
            <h1 className="mt-7 text-[30px] font-semibold leading-none tracking-[-0.035em] text-foreground">
              PaperSage
            </h1>
            <p className="mt-3 text-[13.5px] leading-6 text-muted-foreground">
              智能化科研文献管理工作台
            </p>
          </div>
        </aside>

        <main className="grid min-h-0 place-items-center px-4 sm:px-8 lg:border-l lg:border-border/40">
          <div className="auth-page-content my-auto w-full max-w-[420px]">
            {/* 窄屏品牌块：横向紧凑，低高度也不丢标题 */}
            <div className="mb-5 flex items-center justify-center gap-2.5 lg:hidden">
              <img
                src="/app-icon.png"
                alt=""
                className="size-9 rounded-[10px] shadow ring-1 ring-black/5 dark:ring-white/10"
              />
              <div>
                <h1 className="text-[17px] font-semibold leading-none tracking-tight text-foreground">PaperSage</h1>
                <p className="mt-1 text-[11.5px] text-muted-foreground">智能化科研文献管理工作台</p>
              </div>
            </div>

            <Card className="gap-0 overflow-hidden rounded-[18px] border border-border/60 bg-card/95 py-0 shadow-[0_30px_70px_-34px_rgba(15,23,42,0.55)] ring-1 ring-foreground/5 backdrop-blur-sm">
              <CardContent className="p-0">
                <div className="px-6 pb-5 pt-6 sm:px-7 sm:pt-7">
                  <h2 className="text-[18px] font-semibold tracking-[-0.015em] text-foreground">
                    {mode === 'login' ? '登录 PaperSage' : '创建 PaperSage 账号'}
                  </h2>
                  <p className="mt-1 text-[12.5px] leading-5 text-muted-foreground">
                    {mode === 'login'
                      ? getApiTarget() === 'online'
                        ? '登录后同步你的文献、稿件与写作进度'
                        : '登录本机工作区，稿件保存在本机'
                      : '使用学校邮箱注册，验证后即可开始'}
                  </p>

                  <Tabs
                    value={mode}
                    onValueChange={(v) => {
                      setAuthError('')
                      setNeedsVerify(false)
                      setMode(v as AuthMode)
                    }}
                    className="mt-5 w-full gap-0"
                  >
                    <TabsList className="grid w-full grid-cols-2 rounded-[10px] bg-muted/60 p-[3px]">
                      <TabsTrigger value="login" className="text-[13px] font-medium transition-all duration-200">
                        账号登录
                      </TabsTrigger>
                      <TabsTrigger value="register" className="text-[13px] font-medium transition-all duration-200">
                        注册新账号
                      </TabsTrigger>
                    </TabsList>

                    <SmoothHeight className="pt-5" data-testid="auth-panels">
                      <TabsContent value="login" className="mt-0 space-y-4 outline-none animate-in fade-in slide-in-from-left-2 duration-300">
                        {pendingVerifyEmail && (
                          <div className="flex items-start gap-2.5 rounded-lg border border-primary/20 bg-primary/[0.06] p-3">
                            <MailCheck className="mt-0.5 size-4 flex-none text-primary/70" />
                            <div className="flex-1 text-[12px] leading-relaxed text-foreground/80">
                              验证邮件已发送至 <span className="font-semibold">{pendingVerifyEmail}</span>，
                              请查收并点击邮件中的链接激活账号后再登录（24 小时内有效，注意垃圾箱）。
                              {guestImportPending && <p className="mt-1.5 text-[11px] leading-4 text-muted-foreground">当前浏览器中的游客项目仍保存在本机。验证并登录后，你可以检查内容并确认导入；不会自动上传。</p>}
                              <ResendButton email={pendingVerifyEmail} className="ml-1" />
                            </div>
                          </div>
                        )}
                        {needsVerify && loginForm.getValues('username') && (
                          <div className="text-[11.5px]">
                            <ResendButton email={loginForm.getValues('username')} className="font-medium" />
                          </div>
                        )}
                        {authError && (
                          <Alert variant="destructive" className="rounded-lg border-none bg-destructive/10 py-2.5 text-destructive">
                            <ShieldCheck className="mr-2 h-4 w-4" />
                            <AlertDescription className="text-[12px] font-medium leading-snug">{authError}</AlertDescription>
                          </Alert>
                        )}
                        <form onSubmit={handleLoginSubmit} className="space-y-3.5">
                          <div className="space-y-1.5">
                            <Label htmlFor="username" className={fieldLabelClass}>用户名</Label>
                            <Input
                              id="username"
                              autoComplete="username"
                              autoCapitalize="none"
                              spellCheck={false}
                              {...loginForm.register('username')}
                              placeholder="请输入您的账号"
                              className={fieldInputClass}
                            />
                            {loginForm.formState.errors.username && (
                              <p className="ml-1 text-[11.5px] text-destructive">{loginForm.formState.errors.username.message}</p>
                            )}
                          </div>
                          <div className="space-y-1.5">
                            <div className="flex items-center justify-between">
                              <Label htmlFor="password" className={fieldLabelClass}>密码</Label>
                              <Link to="/auth/forgot-password" className="text-[11.5px] font-medium text-primary/80 transition-colors hover:text-primary">
                                忘记密码？
                              </Link>
                            </div>
                            <div className="relative">
                              <Input
                                id="password"
                                type={showPassword ? 'text' : 'password'}
                                autoComplete="current-password"
                                {...loginForm.register('password')}
                                placeholder="••••••••"
                                className={cn(fieldInputClass, 'pr-10')}
                              />
                              <button
                                type="button"
                                aria-label={showPassword ? '隐藏密码' : '显示密码'}
                                onClick={() => setShowPassword(!showPassword)}
                                className="absolute right-0 top-1/2 flex size-10 -translate-y-1/2 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                              >
                                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                              </button>
                            </div>
                            {loginForm.formState.errors.password && (
                              <p className="ml-1 text-[11.5px] text-destructive">{loginForm.formState.errors.password.message}</p>
                            )}
                          </div>
                          <Button
                            type="submit"
                            disabled={loginMutation.isPending}
                            className="h-12 w-full gap-2 rounded-[10px] text-[13.5px] font-semibold"
                          >
                            {loginMutation.isPending
                              ? <LoaderCircle className="size-4 animate-spin" />
                              : <LogIn className="size-4" />}
                            {loginMutation.isPending ? '正在连接...' : '立即登录'}
                          </Button>
                        </form>

                      </TabsContent>

                      <TabsContent value="register" className="mt-0 space-y-4 outline-none animate-in fade-in slide-in-from-right-2 duration-300">
                        {authError && (
                          <Alert variant="destructive" className="rounded-lg border-none bg-destructive/10 py-2.5 text-destructive">
                            <ShieldCheck className="mr-2 h-4 w-4" />
                            <AlertDescription className="text-[12px] font-medium leading-snug">{authError}</AlertDescription>
                          </Alert>
                        )}
                        <form onSubmit={handleRegisterSubmit} className="space-y-3.5">
                          <div className="space-y-1.5">
                            <Label htmlFor="reg-email" className={fieldLabelClass}>学校邮箱</Label>
                            <Input
                              id="reg-email"
                              type="email"
                              autoComplete="email"
                              autoCapitalize="none"
                              spellCheck={false}
                              {...registerForm.register('email')}
                              placeholder="your-id@hit.edu.cn"
                              className={fieldInputClass}
                            />
                            {registerForm.formState.errors.email && (
                              <p className="ml-1 text-[11.5px] text-destructive">{registerForm.formState.errors.email.message}</p>
                            )}
                            <AllowedDomainsHint />
                          </div>
                          <div className="space-y-1.5">
                            <Label htmlFor="reg-password" className={fieldLabelClass}>设置密码</Label>
                            <Input
                              id="reg-password"
                              type="password"
                              autoComplete="new-password"
                              {...registerForm.register('password')}
                              placeholder="至少 8 位，包含字母与数字"
                              className={fieldInputClass}
                            />
                            {registerForm.formState.errors.password && (
                              <p className="ml-1 text-[11.5px] text-destructive">{registerForm.formState.errors.password.message}</p>
                            )}
                          </div>
                          <div className="space-y-1.5">
                            <Label htmlFor="reg-confirm" className={fieldLabelClass}>确认密码</Label>
                            <Input
                              id="reg-confirm"
                              type="password"
                              autoComplete="new-password"
                              {...registerForm.register('confirmPassword')}
                              placeholder="请再次输入密码"
                              className={fieldInputClass}
                            />
                            {registerForm.formState.errors.confirmPassword && (
                              <p className="ml-1 text-[11.5px] text-destructive">{registerForm.formState.errors.confirmPassword.message}</p>
                            )}
                          </div>
                          <Button
                            type="submit"
                            disabled={registerMutation.isPending}
                            className="h-12 w-full gap-2 rounded-[10px] text-[13.5px] font-semibold"
                          >
                            {registerMutation.isPending
                              ? <LoaderCircle className="size-4 animate-spin" />
                              : <UserPlus className="size-4" />}
                            {registerMutation.isPending ? '正在创建...' : '创建我的账号'}
                          </Button>
                        </form>
                      </TabsContent>
                    </SmoothHeight>
                  </Tabs>

                <div className="mt-4 border-t border-border/40 pt-3">
                  {guestError && (
                    <Alert variant="destructive" className="mb-2 rounded-lg py-2.5">
                      <ShieldCheck className="size-4" />
                      <AlertDescription className="text-[12px]">{guestError}</AlertDescription>
                    </Alert>
                  )}
                  <button
                    type="button"
                    onClick={handleGuestStart}
                    disabled={guestStarting}
                    aria-label="以游客身份体验 PaperSage"
                    className="flex min-h-12 w-full items-center justify-center gap-1.5 rounded-lg text-[12.5px] font-medium text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-wait disabled:opacity-70"
                  >
                    {guestStarting
                      ? <LoaderCircle className="size-3.5 animate-spin" />
                      : <HardDrive className="size-3.5" />}
                    <span>{guestStarting ? '正在准备本机工作区…' : '免注册体验 PaperSage'}</span>
                    {!guestStarting && <ChevronRight className="size-3.5 text-muted-foreground/50" />}
                  </button>
                  <p className="pb-1 text-center text-[11.5px] leading-4 text-muted-foreground">
                    项目仅保存在此设备，可稍后导入账号
                  </p>
                </div>
                </div>
              </CardContent>

              <div className="flex items-center justify-between gap-3 border-t border-border/50 bg-muted/25 px-5 py-3 sm:px-6">
                <p className="text-[10.5px] leading-4 text-muted-foreground">
                  继续操作即表示同意
                  <a href="#" className="mx-1 font-medium text-foreground/80 underline-offset-2 transition-colors hover:text-foreground hover:underline">服务条款</a>
                  和
                  <a href="#" className="ml-1 font-medium text-foreground/80 underline-offset-2 transition-colors hover:text-foreground hover:underline">隐私政策</a>
                </p>
                {!isDesktop() && <ApiTargetToggle prominent />}
              </div>
            </Card>
          </div>
        </main>
      </div>

      {isDesktop() && (
        <>
          <div className="desktop-drag absolute inset-x-0 top-0 z-30 h-9" aria-hidden="true" />
          <DesktopAppMenu className="absolute left-3 top-2.5 z-40" />
          <WindowControls className="absolute right-0 top-0 z-40 h-9" />
        </>
      )}
    </div>
  )
}

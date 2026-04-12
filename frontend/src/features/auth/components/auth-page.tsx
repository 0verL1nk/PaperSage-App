import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight, Sparkles, Eye, EyeOff, ShieldCheck } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useLoginMutation, useRegisterMutation } from '@/features/auth/hooks'
import {
  loginSchema,
  registerSchema,
  type LoginFormValues,
  type RegisterFormValues,
} from '@/features/auth/schema'
import { setSessionTokens, useSessionStore } from '@/shared/stores'

type AuthMode = 'login' | 'register'

function getErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }

  return '操作失败，请稍后重试。'
}

export function AuthPage() {
  const navigate = useNavigate()
  const accessToken = useSessionStore((state) => state.accessToken)
  const loginMutation = useLoginMutation()
  const registerMutation = useRegisterMutation()
  const [mode, setMode] = useState<AuthMode>('login')
  const [authError, setAuthError] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  // Redirect if already logged in
  useEffect(() => {
    if (accessToken) {
      navigate({ to: '/app/projects' })
    }
  }, [accessToken, navigate])

  const loginForm = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      username: '',
      password: '',
    },
  })

  const registerForm = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      username: '',
      password: '',
      confirmPassword: '',
    },
  })

  useEffect(() => {
    if (accessToken) {
      void navigate({
        to: '/',
        replace: true,
      })
    }
  }, [accessToken, navigate])

  const handleModeChange = (nextMode: string) => {
    if (nextMode !== 'login' && nextMode !== 'register') {
      return
    }

    setMode(nextMode)
    setAuthError('')
    setShowPassword(false)
  }

  const handleLoginSubmit = loginForm.handleSubmit(async (values) => {
    setAuthError('')

    try {
      const tokens = await loginMutation.mutateAsync(values)

      setSessionTokens(tokens)
      toast.success('登录成功', {
        description: 'PaperSage 会话已建立。',
      })
      await navigate({
        to: '/',
        replace: true,
      })
    } catch (error) {
      const message = getErrorMessage(error)

      setAuthError(message)
      toast.error('登录失败', {
        description: message,
      })
    }
  })

  const handleRegisterSubmit = registerForm.handleSubmit(async (values) => {
    setAuthError('')

    try {
      await registerMutation.mutateAsync({
        username: values.username,
        password: values.password,
      })

      toast.success('注册成功', {
        description: '账号已创建，请使用新密码登录。',
      })
      setMode('login')
      loginForm.reset({
        username: values.username,
        password: '',
      })
      registerForm.reset({
        username: values.username,
        password: '',
        confirmPassword: '',
      })
    } catch (error) {
      const message = getErrorMessage(error)

      setAuthError(message)
      toast.error('注册失败', {
        description: message,
      })
    }
  })

  const loginErrors = loginForm.formState.errors
  const registerErrors = registerForm.formState.errors

  return (
    <main className="relative flex h-screen items-center justify-center overflow-hidden bg-[#f8f6f2] px-6 py-8 text-foreground">
      {/* Dynamic Background Elements - Enhanced Visibility */}
      <div className="auth-grid absolute inset-0 bg-[linear-gradient(rgba(36,52,89,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(36,52,89,0.08)_1px,transparent_1px)] bg-[size:40px_40px]" />
      
      <div className="auth-mesh absolute -left-20 -top-40 size-[35rem] rounded-full bg-[radial-gradient(circle,rgba(239,188,91,0.4)_0%,rgba(239,188,91,0.15)_45%,transparent_70%)] blur-3xl" />
      <div className="auth-mesh-reverse absolute -right-10 top-20 size-[30rem] rounded-full bg-[radial-gradient(circle,rgba(119,145,220,0.3)_0%,rgba(119,145,220,0.1)_45%,transparent_70%)] blur-3xl" />
      <div className="auth-mesh absolute bottom-[-8rem] left-[15%] size-[28rem] rounded-full bg-[radial-gradient(circle,rgba(255,255,255,0.95)_0%,rgba(255,255,255,0.6)_50%,transparent_80%)] blur-3xl" />
      <div className="auth-mesh-reverse absolute bottom-[-12rem] right-[10%] size-[30rem] rounded-full bg-[radial-gradient(circle,rgba(204,215,236,0.35)_0%,rgba(204,215,236,0.15)_45%,transparent_75%)] blur-3xl" />

      <div className="relative z-10 w-full max-w-[28rem] animate-in fade-in slide-in-from-bottom-8 duration-700 ease-out">
        {/* Shadow Bloom */}
        <div className="absolute inset-x-8 -bottom-8 h-24 rounded-[3rem] bg-slate-900/15 blur-3xl" />
        
        <Card className="overflow-hidden border-white/60 bg-white/80 shadow-2xl backdrop-blur-2xl dark:border-white/10 dark:bg-slate-950/80">
          <CardHeader className="space-y-6 pb-6 pt-8">
            <div className="flex flex-col items-center justify-center space-y-4 text-center">
              <div className="relative">
                <div className="absolute -inset-2 rounded-2xl bg-gradient-to-tr from-primary/50 to-accent/50 blur-xl opacity-50" />
                <div className="relative flex size-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-xl ring-1 ring-white/30">
                  <Sparkles className="size-7" />
                </div>
              </div>
              
              <div className="space-y-1.5">
                <CardTitle className="bg-gradient-to-br from-slate-950 via-slate-800 to-slate-600 bg-clip-text text-3xl font-extrabold tracking-tight text-transparent dark:from-white dark:to-slate-400">
                  PaperSage
                </CardTitle>
                <CardDescription className="text-balance text-sm font-semibold text-slate-500 dark:text-slate-400">
                  智能文献工作台，开启你的科研新视界
                </CardDescription>
              </div>
            </div>

            <Tabs value={mode} onValueChange={handleModeChange} className="w-full">
              <TabsList className="grid h-12 w-full grid-cols-2 rounded-xl bg-slate-200/50 p-1 dark:bg-slate-800/50">
                <TabsTrigger 
                  value="login" 
                  className="rounded-lg text-sm font-bold transition-all data-[state=active]:bg-white data-[state=active]:text-primary data-[state=active]:shadow-md dark:data-[state=active]:bg-slate-950"
                >
                  登录
                </TabsTrigger>
                <TabsTrigger 
                  value="register" 
                  className="rounded-lg text-sm font-bold transition-all data-[state=active]:bg-white data-[state=active]:text-primary data-[state=active]:shadow-md dark:data-[state=active]:bg-slate-950"
                >
                  注册
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </CardHeader>

          <CardContent className="px-8 pb-7 pt-2">
            {authError ? (
              <Alert variant="destructive" className="mb-6 animate-in fade-in zoom-in-95 duration-300">
                <ShieldCheck className="size-4" />
                <AlertTitle className="ml-2">提交失败</AlertTitle>
                <AlertDescription className="ml-2">{authError}</AlertDescription>
              </Alert>
            ) : null}

            <Tabs value={mode}>
              <TabsContent value="login" className="mt-0 focus-visible:outline-none animate-in fade-in slide-in-from-left-4 duration-500">
                <form className="flex flex-col gap-5" onSubmit={handleLoginSubmit}>
                  <FieldGroup className="space-y-4">
                    <Field data-invalid={Boolean(loginErrors.username)}>
                      <FieldLabel htmlFor="login-username" className="text-xs font-semibold uppercase tracking-wider text-slate-500/80">用户名</FieldLabel>
                      <Input
                        id="login-username"
                        autoComplete="username"
                        placeholder="请输入用户名"
                        className="h-11 rounded-xl border-slate-200/60 bg-white/50 px-4 transition-all focus:bg-white focus:ring-4 focus:ring-primary/10 dark:border-slate-800/60 dark:bg-slate-900/50"
                        disabled={loginMutation.isPending}
                        {...loginForm.register('username')}
                      />
                      {loginErrors.username?.message ? (
                        <FieldError className="text-[11px] font-medium">{loginErrors.username.message}</FieldError>
                      ) : null}
                    </Field>

                    <Field data-invalid={Boolean(loginErrors.password)}>
                      <div className="flex items-center justify-between">
                      <FieldLabel htmlFor="login-password" className="text-xs font-semibold uppercase tracking-wider text-slate-500/80">密码</FieldLabel>
                        <button type="button" className="text-xs font-medium text-primary/80 hover:text-primary hover:underline underline-offset-4">
                          忘记密码?
                        </button>
                      </div>
                      <div className="relative">
                        <Input
                          id="login-password"
                          type={showPassword ? 'text' : 'password'}
                          autoComplete="current-password"
                          aria-label="密码"
                          placeholder="请输入密码"
                          className="h-11 rounded-xl border-slate-200/60 bg-white/50 pl-4 pr-11 transition-all focus:bg-white focus:ring-4 focus:ring-primary/10 dark:border-slate-800/60 dark:bg-slate-900/50"
                          disabled={loginMutation.isPending}
                          {...loginForm.register('password')}
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
                        >
                          {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                        </button>
                      </div>
                      {loginErrors.password?.message ? (
                        <FieldError className="text-[11px] font-medium">{loginErrors.password.message}</FieldError>
                      ) : null}
                    </Field>

                    <Button 
                      type="submit" 
                      aria-label="登录"
                      className="mt-2 h-11 w-full rounded-xl bg-primary text-sm font-semibold shadow-lg shadow-primary/20 transition-all hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary/30 active:translate-y-0" 
                      size="lg" 
                      disabled={loginMutation.isPending}
                    >
                      {loginMutation.isPending ? '登录中...' : '登录'}
                      {loginMutation.isPending ? null : <ArrowRight className="ml-2 size-4" />}
                    </Button>
                  </FieldGroup>
                </form>
              </TabsContent>

              <TabsContent value="register" className="mt-0 focus-visible:outline-none animate-in fade-in zoom-in-95 duration-500">
                <form className="flex flex-col gap-5" onSubmit={handleRegisterSubmit}>
                  <FieldGroup className="space-y-4">
                    <Field data-invalid={Boolean(registerErrors.username)}>
                      <FieldLabel htmlFor="register-username" className="text-xs font-semibold uppercase tracking-wider text-slate-500/80">用户名</FieldLabel>
                      <Input
                        id="register-username"
                        autoComplete="username"
                        placeholder="请创建用户名"
                        className="h-11 rounded-xl border-slate-200/60 bg-white/50 px-4 transition-all focus:bg-white focus:ring-4 focus:ring-primary/10 dark:border-slate-800/60 dark:bg-slate-900/50"
                        disabled={registerMutation.isPending}
                        {...registerForm.register('username')}
                      />
                      {registerErrors.username?.message ? (
                        <FieldError className="text-[11px] font-medium">{registerErrors.username.message}</FieldError>
                      ) : null}
                    </Field>

                    <Field data-invalid={Boolean(registerErrors.password)}>
                      <FieldLabel htmlFor="register-password" className="text-xs font-semibold uppercase tracking-wider text-slate-500/80">密码</FieldLabel>
                      <div className="relative">
                        <Input
                          id="register-password"
                          type={showPassword ? 'text' : 'password'}
                          autoComplete="new-password"
                          aria-label="密码"
                          placeholder="至少 8 位，包含字母和数字"
                          className="h-11 rounded-xl border-slate-200/60 bg-white/50 pl-4 pr-11 transition-all focus:bg-white focus:ring-4 focus:ring-primary/10 dark:border-slate-800/60 dark:bg-slate-900/50"
                          disabled={registerMutation.isPending}
                          {...registerForm.register('password')}
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
                        >
                          {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                        </button>
                      </div>
                      {registerErrors.password?.message ? (
                        <FieldError className="text-[11px] font-medium">{registerErrors.password.message}</FieldError>
                      ) : null}
                    </Field>

                    <Field data-invalid={Boolean(registerErrors.confirmPassword)}>
                      <FieldLabel htmlFor="register-confirm-password" className="text-xs font-semibold uppercase tracking-wider text-slate-500/80">确认密码</FieldLabel>
                      <Input
                        id="register-confirm-password"
                        type="password"
                        autoComplete="new-password"
                        aria-label="确认密码"
                        placeholder="请再次输入密码"
                        className="h-11 rounded-xl border-slate-200/60 bg-white/50 px-4 transition-all focus:bg-white focus:ring-4 focus:ring-primary/10 dark:border-slate-800/60 dark:bg-slate-900/50"
                        disabled={registerMutation.isPending}
                        {...registerForm.register('confirmPassword')}
                      />
                      {registerErrors.confirmPassword?.message ? (
                        <FieldError className="text-[11px] font-medium">{registerErrors.confirmPassword.message}</FieldError>
                      ) : null}
                    </Field>

                    <Button
                      type="submit"
                      aria-label="创建账号"
                      className="mt-2 h-11 w-full rounded-xl bg-primary text-sm font-semibold shadow-lg shadow-primary/20 transition-all hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary/30 active:translate-y-0"
                      size="lg"
                      disabled={registerMutation.isPending}
                    >
                      {registerMutation.isPending ? '创建中...' : '创建账号'}
                      {registerMutation.isPending ? null : <ArrowRight className="ml-2 size-4" />}
                    </Button>
                  </FieldGroup>
                </form>
              </TabsContent>
            </Tabs>
          </CardContent>
          
          <div className="border-t border-slate-100/50 bg-slate-50/50 px-8 py-4 text-center dark:border-slate-800/50 dark:bg-slate-900/50">
            <p className="text-xs text-slate-400">
              继续即代表您同意我们的 <a href="#" className="underline underline-offset-4 hover:text-primary">服务协议</a> 和 <a href="#" className="underline underline-offset-4 hover:text-primary">隐私政策</a>
            </p>
          </div>
        </Card>
      </div>
    </main>
  )
}

import { useEffect, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, BookOpenText, Check, HardDrive, LoaderCircle, ShieldAlert, Sparkles } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { initializeGuestWorkspace } from '@/services/guest-local-workspace'
import { activateLocalGuestSession, useSessionStore } from '@/store'

export function GuestWorkspacePage() {
  const navigate = useNavigate()
  const accessToken = useSessionStore((state) => state.accessToken)
  const isGuest = useSessionStore((state) => state.isGuest)
  const [error, setError] = useState('')
  const [isStarting, setIsStarting] = useState(false)

  useEffect(() => {
    if (accessToken || (isGuest && !isStarting)) void navigate({ to: '/app/projects' })
  }, [accessToken, isGuest, isStarting, navigate])

  const startGuestWorkspace = async () => {
    setError('')
    setIsStarting(true)
    try {
      await initializeGuestWorkspace()
      activateLocalGuestSession()
      await navigate({ to: '/app/projects' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '无法初始化本地游客工作区，请检查浏览器存储设置后重试。')
    } finally {
      setIsStarting(false)
    }
  }

  return (
    <main className="relative flex min-h-screen overflow-hidden bg-background text-foreground">
      <div aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="auth-mesh absolute -left-[24%] -top-[30%] size-[min(90vw,900px)] rounded-full bg-zinc-400/10 blur-[120px] dark:bg-zinc-500/10" />
        <div className="auth-mesh-reverse absolute -bottom-[35%] -right-[20%] size-[min(75vw,760px)] rounded-full bg-amber-300/15 blur-[120px] dark:bg-amber-500/10" />
        <div className="auth-grid absolute inset-0" />
        <div className="absolute inset-0 bg-noise opacity-[0.025] mix-blend-overlay dark:opacity-[0.04]" />
      </div>

      <div className="relative z-10 mx-auto flex w-full max-w-6xl flex-col px-4 py-5 sm:px-8 sm:py-8 lg:px-10">
        <header className="flex items-center justify-between gap-3">
          <Link to="/auth" className="inline-flex min-h-12 items-center gap-2 rounded-full px-3 text-sm text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground">
            <ArrowLeft className="size-4" />
            <span>返回登录</span>
          </Link>
          <div className="flex items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-xl bg-foreground text-background"><span className="text-lg font-bold italic">S</span></span>
            <span className="text-sm font-semibold tracking-tight">PaperSage</span>
          </div>
          <span className="hidden w-[116px] sm:block" aria-hidden="true" />
        </header>

        <div className="grid flex-1 content-center gap-8 py-8 md:grid-cols-[minmax(0,1.1fr)_minmax(320px,0.9fr)] md:items-center md:gap-12 lg:gap-20">
          <section className="mx-auto w-full max-w-xl md:mx-0">
            <Badge variant="outline" className="mb-5 gap-1.5 rounded-full border-primary/20 bg-primary/5 px-3 py-1 text-primary">
              <Sparkles className="size-3.5" /> 本机游客工作区
            </Badge>
            <h1 className="text-balance text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl lg:leading-[1.12]">
              先开始写作，<br className="hidden sm:block" />准备好后再注册
            </h1>
            <p className="mt-4 max-w-lg text-pretty text-sm leading-7 text-muted-foreground sm:text-base">
              无需账号即可在当前浏览器体验 PaperSage。游客项目保存在本机浏览器，不会上传到云端；登录后可先检查内容，再选择导入账号。
            </p>

            <div className="mt-7 grid gap-3 sm:grid-cols-2">
              <div className="flex gap-3 rounded-2xl border border-border/60 bg-card/65 p-4 backdrop-blur-sm">
                <HardDrive className="mt-0.5 size-4 shrink-0 text-primary" />
                <div><p className="text-sm font-medium">仅此设备保存</p><p className="mt-1 text-xs leading-5 text-muted-foreground">项目与内容写入浏览器本地数据库，不会自动同步。</p></div>
              </div>
              <div className="flex gap-3 rounded-2xl border border-border/60 bg-card/65 p-4 backdrop-blur-sm">
                <BookOpenText className="mt-0.5 size-4 shrink-0 text-primary" />
                <div><p className="text-sm font-medium">可以转入账号</p><p className="mt-1 text-xs leading-5 text-muted-foreground">登录后先查看待导入内容，确认后再上传到账号。</p></div>
              </div>
            </div>

            <ul className="mt-6 grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
              <li className="flex items-center gap-2"><Check className="size-4 text-emerald-600" />可创建并编辑真实本地项目</li>
              <li className="flex items-center gap-2"><Check className="size-4 text-emerald-600" />导入前由你确认内容</li>
              <li className="flex items-center gap-2"><Check className="size-4 text-emerald-600" />失败可保留本机数据后重试</li>
              <li className="flex items-center gap-2"><Check className="size-4 text-emerald-600" />访客数据与云端账号隔离</li>
            </ul>
          </section>

          <Card className="mx-auto w-full max-w-md overflow-hidden rounded-[1.75rem] border-border/70 bg-card/90 shadow-2xl shadow-black/5 backdrop-blur-xl md:mx-0">
            <CardHeader className="space-y-3 p-6 sm:p-8">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary"><BookOpenText className="size-5" /></div>
              <div>
                <CardTitle className="text-xl">开启本地游客工作区</CardTitle>
                <CardDescription className="mt-2 leading-6">游客模式不创建服务端账号或会话，项目内容留在当前浏览器。</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="space-y-5 px-6 pb-6 sm:px-8 sm:pb-8">
              {error && <Alert variant="destructive" className="rounded-xl"><ShieldAlert className="size-4" /><AlertTitle>无法开启游客模式</AlertTitle><AlertDescription>{error}</AlertDescription></Alert>}
              <Alert className="rounded-xl border-amber-500/30 bg-amber-500/5">
                <ShieldAlert className="size-4 text-amber-700" />
                <AlertTitle>请留意本地数据风险</AlertTitle>
                <AlertDescription className="leading-5">清除浏览器站点数据、卸载浏览器或更换设备可能导致游客内容无法恢复。重要内容请尽早注册并完成账号导入。</AlertDescription>
              </Alert>
              <Button onClick={startGuestWorkspace} disabled={isStarting} className="min-h-12 w-full rounded-xl text-sm font-semibold" size="lg">
                {isStarting ? <><LoaderCircle className="mr-2 size-4 animate-spin" />正在检查本机存储…</> : <>开始游客体验<ArrowRight className="ml-2 size-4" /></>}
              </Button>
              <p className="text-center text-xs leading-5 text-muted-foreground">
                已准备好长期使用？{' '}
                <Link to="/auth" className="inline-flex min-h-11 items-center rounded px-2 font-semibold text-foreground underline-offset-4 hover:underline">注册或登录</Link>
              </p>
            </CardContent>
          </Card>
        </div>

        <footer className="flex flex-col items-center justify-between gap-2 border-t border-border/40 pt-4 text-center text-[11px] text-muted-foreground sm:flex-row sm:text-left">
          <span>PaperSage · 科研写作工作台</span>
          <span>游客工作区只保存在本机浏览器，不代表云端已保存。</span>
        </footer>
      </div>
    </main>
  )
}

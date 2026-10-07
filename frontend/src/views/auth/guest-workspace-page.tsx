import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { HardDrive, LoaderCircle, ShieldAlert } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { initializeGuestWorkspace } from '@/services/guest-local-workspace'
import { activateLocalGuestSession, useSessionStore } from '@/store'

/** Direct /guest visits initialize storage and enter the workspace without an onboarding screen. */
export function GuestWorkspacePage() {
  const navigate = useNavigate()
  const accessToken = useSessionStore((state) => state.accessToken)
  const isGuest = useSessionStore((state) => state.isGuest)
  const [error, setError] = useState('')
  const [isStarting, setIsStarting] = useState(true)
  const started = useRef(false)

  const startGuestWorkspace = useCallback(async () => {
    if (started.current) return
    started.current = true
    setError('')
    setIsStarting(true)
    try {
      await initializeGuestWorkspace()
      activateLocalGuestSession()
      await navigate({ to: '/app/projects' })
    } catch (cause) {
      started.current = false
      setError(cause instanceof Error ? cause.message : '无法开启本机游客工作区，请检查浏览器存储设置后重试。')
    } finally {
      setIsStarting(false)
    }
  }, [navigate])

  useEffect(() => {
    if (accessToken || isGuest) {
      if (!started.current) void navigate({ to: '/app/projects' })
      return
    }
    void startGuestWorkspace()
  }, [accessToken, isGuest, navigate, startGuestWorkspace])

  return (
    <main className="relative flex min-h-[100dvh] items-center justify-center overflow-hidden bg-background px-5 text-foreground">
      <div aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="auth-aurora auth-mesh absolute -left-[22%] -top-[25%] size-[80vw] rounded-full blur-[96px]" />
        <div className="auth-aurora-warm auth-mesh-reverse absolute -bottom-[20%] -right-[20%] size-[75vw] rounded-full blur-[100px]" />
        <div className="auth-grid absolute inset-0" />
      </div>
      <section aria-live="polite" className="relative z-10 w-full max-w-sm text-center">
        <div className="mx-auto mb-5 flex size-12 items-center justify-center rounded-2xl border border-primary/15 bg-background/75 text-primary shadow-sm backdrop-blur-md">
          {isStarting ? <LoaderCircle className="size-5 animate-spin" /> : <HardDrive className="size-5" />}
        </div>
        <h1 className="text-lg font-semibold">{isStarting ? '正在准备本机工作区' : '无法开启游客工作区'}</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {isStarting ? '检查浏览器本地存储后将直接进入项目列表…' : '你的内容不会上传；检查存储设置后可以重试。'}
        </p>
        {error && (
          <Alert variant="destructive" className="mt-5 text-left">
            <ShieldAlert className="size-4" />
            <AlertTitle>本机存储不可用</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {!isStarting && error && <Button className="mt-5 min-h-11" onClick={() => void startGuestWorkspace()}>重试</Button>}
      </section>
    </main>
  )
}

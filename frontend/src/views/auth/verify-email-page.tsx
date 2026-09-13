/**
 * 邮箱验证结果页：邮件链接 /auth/verify?token=... 落地。
 * 调后端一次性验证端点，展示成功/失败状态与去向。
 */
import { useEffect, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { CheckCircle2, XCircle, Loader2, MailCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { verifyEmail } from '@/services/auth-api'

type VerifyState = 'pending' | 'success' | 'error'

export function VerifyEmailPage() {
  const navigate = useNavigate()
  const search = useSearch({ strict: false }) as { token?: string }
  const [state, setState] = useState<VerifyState>('pending')
  const [message, setMessage] = useState('')

  useEffect(() => {
    const token = search.token
    let cancelled = false
    const fail = (msg: string) => {
      if (cancelled) return
      setState('error')
      setMessage(msg)
    }
    if (!token) {
      fail('缺少验证参数')
      return
    }
    verifyEmail(token)
      .then(() => { if (!cancelled) setState('success') })
      .catch((err) => fail(err instanceof Error ? err.message : '验证链接无效或已过期'))
    return () => { cancelled = true }
  }, [search.token])

  return (
    <div className="flex h-screen w-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm rounded-3xl border border-border/40 bg-card p-10 text-center shadow-sm">
        <div className="mx-auto mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/5 text-primary/70">
          {state === 'pending' && <Loader2 className="h-8 w-8 animate-spin stroke-[1.5]" />}
          {state === 'success' && <CheckCircle2 className="h-8 w-8 stroke-[1.5] text-green-600" />}
          {state === 'error' && <XCircle className="h-8 w-8 stroke-[1.5] text-destructive" />}
        </div>
        <h2 className="mb-2 text-lg font-semibold text-foreground">
          {state === 'pending' && '正在验证…'}
          {state === 'success' && '邮箱验证成功'}
          {state === 'error' && '验证失败'}
        </h2>
        <p className="mb-8 text-[13px] leading-relaxed text-muted-foreground">
          {state === 'pending' && '请稍候，正在完成验证。'}
          {state === 'success' && '你的账号已激活，现在可以使用学校邮箱登录 PaperSage。'}
          {state === 'error' && (message || '验证链接无效或已过期，请重新注册或重发验证邮件。')}
        </p>
        {state !== 'pending' && (
          <Button
            size="sm"
            className="rounded-full px-6"
            onClick={() => navigate({ to: '/auth' })}
          >
            <MailCheck className="mr-1.5 h-3.5 w-3.5" />
            去登录
          </Button>
        )}
      </div>
    </div>
  )
}

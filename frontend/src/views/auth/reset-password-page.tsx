import { useState, type FormEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowLeft, KeyRound, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { confirmPasswordReset } from '@/services/auth-api'

function takeResetTokenFromFragment(): string {
  if (typeof window === 'undefined') return ''
  const token = new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
  if (token) window.history.replaceState(window.history.state, '', `${window.location.pathname}${window.location.search}`)
  return token
}

export function ResetPasswordPage() {
  const [token] = useState(takeResetTokenFromFragment)
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [pending, setPending] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (!token) {
      setError('重置链接缺少安全凭据，请重新申请。')
      return
    }
    if (password.length < 8) {
      setError('密码至少需要 8 位。')
      return
    }
    if (password !== confirmation) {
      setError('两次输入的密码不一致。')
      return
    }
    setPending(true)
    try {
      await confirmPasswordReset(token, password)
      setPassword('')
      setConfirmation('')
      setDone(true)
    } catch (cause) {
      setError(cause instanceof Error && (cause.message.includes('password reset link') || cause.message.includes('密码重置链接'))
        ? '链接无效、已过期或已使用，请重新申请。'
        : '暂时无法重置密码，请稍后重试。')
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="flex min-h-[100dvh] items-center justify-center bg-background px-4 py-8 sm:px-6">
      <Card className="w-full max-w-md overflow-hidden rounded-[2rem] shadow-xl">
        <CardContent className="space-y-6 p-6 sm:p-9">
          <div className="mx-auto flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            {done ? <ShieldCheck aria-hidden className="size-6" /> : <KeyRound aria-hidden className="size-6" />}
          </div>
          <header className="space-y-2 text-center">
            <h1 className="text-2xl font-semibold tracking-tight">{done ? '密码已更新' : '设置新密码'}</h1>
            <p className="text-sm leading-6 text-muted-foreground">
              {done
                ? '出于安全考虑，你需要使用新密码重新登录。其他已登录会话也已失效。'
                : '设置至少 8 位的新密码。重置链接仅可使用一次，有效期为 15 分钟。'}
            </p>
          </header>

          {done ? (
            <Button asChild className="h-12 w-full rounded-xl"><Link to="/auth">前往登录</Link></Button>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="new-password">新密码</Label>
                <Input
                  id="new-password"
                  type="password"
                  autoComplete="new-password"
                  minLength={8}
                  required
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="h-12 rounded-xl px-4"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="confirm-new-password">确认新密码</Label>
                <Input
                  id="confirm-new-password"
                  type="password"
                  autoComplete="new-password"
                  minLength={8}
                  required
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  className="h-12 rounded-xl px-4"
                />
              </div>
              {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="h-12 w-full rounded-xl" disabled={pending || !token}>
                {pending ? '正在更新…' : '更新密码'}
              </Button>
              {!token && <p className="text-center text-sm text-muted-foreground">链接无效或缺少凭据，请重新申请重置链接。</p>}
            </form>
          )}

          {!done && (
            <Button asChild variant="ghost" className="min-h-11 w-full rounded-xl">
              <Link to="/auth/forgot-password"><ArrowLeft aria-hidden className="mr-2 size-4" />重新申请链接</Link>
            </Button>
          )}
        </CardContent>
      </Card>
    </main>
  )
}

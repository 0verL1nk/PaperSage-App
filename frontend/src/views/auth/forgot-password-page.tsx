import { useState, type FormEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowLeft, Mail, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { requestPasswordReset } from '@/services/auth-api'

export function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const [pending, setPending] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setPending(true)
    try {
      await requestPasswordReset(email.trim())
      setSubmitted(true)
    } catch {
      setError('暂时无法处理请求，请稍后再试。')
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="flex min-h-[100dvh] items-center justify-center bg-background px-4 py-8 sm:px-6">
      <Card className="w-full max-w-md overflow-hidden rounded-[2rem] shadow-xl">
        <CardContent className="space-y-6 p-6 sm:p-9">
          <div className="mx-auto flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            {submitted ? <Mail aria-hidden className="size-6" /> : <Send aria-hidden className="size-6" />}
          </div>
          <header className="space-y-2 text-center">
            <h1 className="text-2xl font-semibold tracking-tight">找回密码</h1>
            <p className="text-sm leading-6 text-muted-foreground">
              输入注册并验证过的邮箱，我们会发送一封限时重置链接。
            </p>
          </header>

          {submitted ? (
            <div role="status" className="space-y-4 rounded-2xl border border-border bg-muted/40 p-4 text-center text-sm leading-6">
              <p>如果该邮箱已验证并关联 PaperSage 账号，我们会发送密码重置链接。请检查收件箱和垃圾邮件文件夹。</p>
              <p className="text-muted-foreground">链接 15 分钟内有效；没有收到邮件时，可以稍后再试。</p>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="recovery-email">邮箱地址</Label>
                <Input
                  id="recovery-email"
                  type="email"
                  autoComplete="email"
                  inputMode="email"
                  required
                  maxLength={255}
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="name@example.com"
                  className="h-12 rounded-xl px-4"
                />
              </div>
              {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="h-12 w-full rounded-xl" disabled={pending}>
                {pending ? '正在提交…' : '发送重置链接'}
              </Button>
            </form>
          )}

          <Button asChild variant="ghost" className="min-h-11 w-full rounded-xl">
            <Link to="/auth"><ArrowLeft aria-hidden className="mr-2 size-4" />返回登录</Link>
          </Button>
        </CardContent>
      </Card>
    </main>
  )
}

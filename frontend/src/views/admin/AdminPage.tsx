/**
 * 管理后台 — 系统设置（仅 superuser）：
 * 注册邮箱域白名单（system_settings.allowed_email_domains，DB 持久化，
 * 注册页实时读取）。空列表 = 不限制。
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ShieldCheck, Plus, X, Loader2 } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getSystemSettings, putSystemSettings } from '@/services/admin-api'

export function AdminPage() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getSystemSettings,
  })
  // 本地编辑副本（null = 未改动，展示服务端值）
  const [editDomains, setEditDomains] = useState<string[] | null>(null)
  const domains = editDomains ?? data?.allowed_email_domains ?? []
  const [draft, setDraft] = useState('')

  const save = useMutation({
    mutationFn: () => putSystemSettings({ allowed_email_domains: domains }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-settings'] })
      qc.invalidateQueries({ queryKey: ['auth-email-domains'] })
      toast.success('已保存', { description: '注册页的域名提示将即时更新。' })
    },
    onError: () => toast.error('保存失败'),
  })

  const addDomain = () => {
    const v = draft.trim().toLowerCase().replace(/^\*\./, '*.')
    if (!v || domains.includes(v)) return
    if (!/^(\*\.)?[a-z0-9.-]+\.[a-z]{2,}$/.test(v)) {
      toast.error('格式不正确', { description: '示例：hit.edu.cn 或 *.hit.edu.cn' })
      return
    }
    setEditDomains([...domains, v])
    setDraft('')
  }

  if (isLoading) {
    return <div className="flex h-full items-center justify-center"><Loader2 className="size-6 animate-spin text-muted-foreground" /></div>
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6 p-6">
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-5 text-primary/70" />
        <h1 className="text-lg font-semibold">系统管理</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">注册邮箱域白名单</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-xs leading-relaxed text-muted-foreground">
            限制新用户注册使用的邮箱域名。<span className="font-medium">hit.edu.cn</span> 允许该域及全部子域；
            <span className="font-medium">*.hit.edu.cn</span> 仅允许子域。列表为空 = 不限制。
          </p>

          <div className="flex gap-2">
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && addDomain()}
              placeholder="hit.edu.cn"
              className="h-9"
            />
            <Button size="sm" variant="outline" className="h-9 rounded-lg" onClick={addDomain}>
              <Plus className="mr-1 size-3.5" />添加
            </Button>
          </div>

          {domains.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {domains.map((d) => (
                <span
                  key={d}
                  className="inline-flex items-center gap-1.5 rounded-full bg-muted px-3 py-1 text-xs font-medium"
                >
                  {d}
                  <button
                    type="button"
                    onClick={() => setEditDomains(domains.filter((x) => x !== d))}
                    className="text-muted-foreground/60 hover:text-destructive transition-colors"
                    aria-label={`移除 ${d}`}
                  >
                    <X className="size-3" />
                  </button>
                </span>
              ))}
            </div>
          )}

          <div className="flex justify-end pt-2">
            <Button size="sm" className="px-4" disabled={save.isPending} onClick={() => save.mutate()}>
              {save.isPending ? '保存中…' : '保存'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

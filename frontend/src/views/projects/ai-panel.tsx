import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Loader2, Plus, Send, Sparkles, Square, X } from 'lucide-react'
import { toast } from 'sonner'
import { useNavigate } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ModelSelector } from '@/views/settings/model-selector'
import { useSettingsStore } from '@/store/settings-store'
import { useSessionStore } from '@/store/session-store'
import { useChatStore, type ChatMessage, type ChatPart } from '@/store/chat-store'
import * as agentApi from '@/services/agent-api'
import { latexKeys } from '@/lib/query-keys'
import { writingStateKeys } from '@/services/writing-state-api'
import { cn } from '@/lib/utils'

/**
 * AI 助手面板。
 *
 * 三层信息密度（参考架构 §5.4）：对话主线 → 消息内活动（工具卡片/子 agent 活动）
 * → 按需展开的执行细节。默认简洁，工具细节折叠起来，不把执行轨迹倒进主线。
 */
export function AiPanel({ projectId, mode = 'default' }: { projectId: string; mode?: 'default' | 'preparation' }) {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const isGuest = useSessionStore((state) => state.isGuest)
  const { settings, fetchSettings, patchCategory, pendingAgentModelChanges, error: settingsError } = useSettingsStore()
  const savingModel = pendingAgentModelChanges > 0
  const agent = settings?.agent
  const providers = agent?.providers ?? []
  const hasProvider = providers.length > 0

  // 面板必须自己加载 settings：它原先依赖 ModelSelector 去加载，而 ModelSelector
  // 只在 hasProvider 为真时才渲染——于是「没设置 → 显示未配置 → 不加载设置」成了死循环，
  // 用户打开面板永远看到「尚未配置 AI 模型」。（e2e 抓到的真 bug。）
  useEffect(() => {
    if (!settings && !isGuest) void fetchSettings().catch(() => {})
  }, [settings, fetchSettings, isGuest])

  const chat = useChatStore()
  const queryClient = useQueryClient()
  const [input, setInput] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)
  const selectedProvider = agent?.active_provider_id
    ? providers.find((p) => p.id === agent.active_provider_id)
    : providers.find((p) => p.api_key?.trim() && p.base_url?.trim() && p.models.length > 0)
  const bottomRef = useRef<HTMLDivElement | null>(null)

  // 会话列表：切会话从这里进来
  const convQuery = useQuery({
    queryKey: ['agent-conversations', projectId],
    queryFn: () => agentApi.listConversations(projectId),
    enabled: !isGuest && !!projectId,
  })
  const conversations = isGuest ? [] : convQuery.data ?? []
  const hasHistory = chat.messages.length > 0 || conversations.length > 0

  const activeConversationId = chat.conversationId ?? conversations[0]?.id ?? null
  const processedWritingStateTools = useRef(new Set<string>())

  // 打开面板即接上最近一次会话（没有就等用户发第一条）
  useEffect(() => {
    if (isGuest || !activeConversationId || chat.conversationId === activeConversationId) return
    void chat.open(activeConversationId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeConversationId])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [chat.messages])

  useEffect(() => {
    if (mode !== 'preparation') return
    for (const message of chat.messages) {
      for (const part of message.parts) {
        const toolName = part.meta?.toolName
        if (part.type !== 'tool' || part.status !== 'succeeded' || toolName !== 'save_writing_preparation' || processedWritingStateTools.current.has(part.id)) continue
        processedWritingStateTools.current.add(part.id)
        void queryClient.invalidateQueries({ queryKey: writingStateKeys.project(projectId) })
      }
    }
  }, [chat.messages, mode, projectId, queryClient])

  const send = async (message = input) => {
    const content = message.trim()
    // 同一 tick 内设置可能已变（保存中/最后一个 provider 被删），必须读最新 store 而非闭包里的渲染值。
    const latest = useSettingsStore.getState()
    if (!content || chat.streaming || latest.pendingAgentModelChanges > 0 || !latest.settings?.agent.providers.length) return
    if (message === input) setInput('')
    // 第一条消息会在 store 里顺带建出会话：把会话列表拉一次，
    // 否则下拉框里看不到刚建的这个会话。
    const firstMessage = !activeConversationId
    try {
      await chat.send(projectId, content)
      if (firstMessage) await convQuery.refetch()
    } catch (e) {
      // 发送失败（含会话版本冲突 409）：刷新会话并把错误显示出来，不静默吞掉
      void convQuery.refetch()
      useChatStore.setState({ error: e instanceof Error ? e.message : '发送失败' })
    }
  }

  if (isGuest) {
    return (
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        <PanelHeader />
        <div className="flex-1 flex items-center justify-center p-6">
          <div className="max-w-[240px] text-center">
            <Sparkles size={18} className="mx-auto mb-4 text-muted-foreground" />
            <p className="text-[13px] font-medium mb-1">登录后可使用 AI 助手</p>
            <p className="text-[11px] text-muted-foreground leading-relaxed mb-4">游客项目只保存在本浏览器，AI 对话需要登录账号。</p>
            <Button size="sm" className="rounded-xl" onClick={() => void navigate({ to: '/auth' })}>
              去登录
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!settings && !hasHistory) {
    return (
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        <PanelHeader />
        <div className="flex-1 flex flex-col items-center justify-center gap-3 p-6">
          {settingsError ? <>
            <p role="alert" className="text-sm text-destructive">设置加载失败，请重试。</p>
            <Button size="sm" onClick={() => void fetchSettings().catch(() => {})}>重试</Button>
          </> : <p className="text-sm text-muted-foreground">正在加载模型设置…</p>}
        </div>
      </div>
    )
  }

  if (!hasProvider && !chat.streaming && !hasHistory && !convQuery.isError) {
    // 未配置模型：引导去设置，而不是给一个发不出去的空输入框
    return (
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        <PanelHeader />
        <div className="flex-1 flex items-center justify-center p-6">
          <div className="max-w-[240px] text-center">
            <div className="w-12 h-12 rounded-2xl bg-muted flex items-center justify-center mx-auto mb-4">
              <Sparkles size={18} className="text-muted-foreground" />
            </div>
            <p className="text-[13px] font-medium mb-1">尚未配置 AI 模型</p>
            <p className="text-[11px] text-muted-foreground leading-relaxed mb-4">
              {mode === 'preparation' ? '配置模型后，研究顾问才能通过对话帮你梳理并保存研究计划。' : '在设置里添加模型服务商后即可开始对话'}
            </p>
            <Button size="sm" className="rounded-xl" onClick={() => void navigate({ to: '/app/settings' })}>
              去设置
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col h-full overflow-hidden">
      <PanelHeader
        right={
          <>
            <DropdownMenu open={pickerOpen} onOpenChange={setPickerOpen}>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs text-muted-foreground max-w-[120px]">
                  <span className="truncate">
                    {conversations.find((c) => c.id === activeConversationId)?.title ?? '新对话'}
                  </span>
                  <ChevronDown className="size-3 shrink-0" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-56">
                <DropdownMenuLabel>会话</DropdownMenuLabel>
                {conversations.map((c) => (
                  <DropdownMenuItem key={c.id} onSelect={() => void chat.open(c.id)}>
                    <span className="truncate">{c.title}</span>
                  </DropdownMenuItem>
                ))}
                <DropdownMenuItem onSelect={() => chat.reset()}>
                  <Plus className="size-3 mr-1" /> 新对话
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            {activeConversationId && (
              <Button
                variant="ghost"
                size="icon-xs"
                className="h-7 w-7 rounded-md text-muted-foreground"
                title="删除该会话"
                onClick={async () => {
                  await agentApi.deleteConversation(activeConversationId)
                  chat.reset()
                  await convQuery.refetch()
                }}
              >
                <X size={13} />
              </Button>
            )}
          </>
        }
        model={hasProvider ? (
          <ModelSelector
            variant="compact"
            value={{ providerId: selectedProvider?.id ?? agent?.active_provider_id ?? '', model: selectedProvider?.active_model === undefined ? selectedProvider?.models.at(-1) ?? '' : selectedProvider.active_model || '' }}
            disabled={savingModel || chat.streaming}
            onChange={(sel) => {
              void patchCategory('agent', { model_selection: { provider_id: sel.providerId, model: sel.model } })
                .catch((error: unknown) => {
                  if (!(error instanceof DOMException && error.name === 'AbortError')) {
                    toast.error(t('agent.modelSaveFailed'))
                  }
                })
            }}
          />
        ) : undefined}
      />

      {!settings && settingsError && (
        <div className="px-4 py-2 flex items-center gap-2 text-xs text-destructive">
          设置加载失败，请重试。
          <Button size="sm" variant="outline" onClick={() => void fetchSettings().catch(() => {})}>重试设置</Button>
        </div>
      )}
      {settings && !hasProvider && !chat.streaming && (
        <div className="px-4 py-2 flex items-center gap-2 text-xs text-muted-foreground">
          尚未配置 AI 模型
          <Button size="sm" variant="outline" onClick={() => void navigate({ to: '/app/settings' })}>去设置</Button>
        </div>
      )}
      {convQuery.isError && (
        <div className="px-4 py-2 flex items-center gap-2 text-xs text-destructive">
          会话加载失败，请重试。
          <Button size="sm" variant="outline" onClick={() => void convQuery.refetch()}>重试会话</Button>
        </div>
      )}

      {savingModel && <p className="px-4 py-2 text-xs text-muted-foreground">{t('agent.savingModelSettings')}</p>}
      <ScrollArea className="flex-1">
        <div className="p-4 space-y-4">
          {chat.messages.length === 0 && (
            <div className="bg-primary/5 rounded-2xl p-4 border border-primary/10 text-[12px] leading-relaxed relative">
              <div className="absolute -top-2 left-4 px-2 bg-background border border-primary/10 rounded-full text-[9px] font-black text-primary uppercase shadow-sm">
                AI
              </div>
              {mode === 'preparation'
                ? '告诉我你感兴趣的方向、已有想法或手头材料。我会一次聚焦一个问题，区分事实与假设，并在你确认后保存研究蓝图。'
                : '可以让我查看你的文件与资料、检索文献，或者对稿件提出修改建议。'}
              {mode === 'preparation' && (
                <div className="mt-3 flex flex-wrap gap-2">
                  {[
                    '我还没确定选题，想从兴趣方向开始梳理。',
                    '我有一个初步想法，帮我判断怎么形成研究问题。',
                    '我有任务书或导师要求，想一起拆解。',
                  ].map((prompt) => <Button key={prompt} variant="outline" size="sm" disabled={chat.streaming || savingModel || !hasProvider} className="h-auto min-h-9 whitespace-normal rounded-xl px-3 py-2 text-left text-[11px]" onClick={() => void send(prompt)}>{prompt}</Button>)}
                </div>
              )}
            </div>
          )}
          {chat.messages.map((m) => (
            <MessageRow key={m.id} message={m} projectId={projectId} />
          ))}
          {chat.error && <p className="text-[11px] text-destructive">{chat.error}</p>}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>

      <div className="p-4 border-t border-border/40 bg-muted/5 flex-none">
        <div className="relative group">
          <textarea
            value={input}
            disabled={!hasProvider}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void send()
              }
            }}
            placeholder={mode === 'preparation' ? '说说你的研究兴趣、初步想法或遇到的困难…' : '问点什么…'}
            className="w-full rounded-2xl border border-border/60 bg-background p-4 pr-12 text-[12px] focus:outline-none focus:ring-2 focus:ring-primary/10 resize-none h-28"
          />
          <Button
            size="icon-xs"
            className="absolute right-3 bottom-3 rounded-xl shadow-lg shadow-primary/20"
            disabled={!chat.streaming && (savingModel || !hasProvider || !input.trim())}
            onClick={() => (chat.streaming ? void chat.cancel() : void send())}
            title={chat.streaming ? '停止' : '发送'}
          >
            {chat.streaming ? <Square size={12} /> : <Send size={14} />}
          </Button>
        </div>
      </div>
    </div>
  )
}

function PanelHeader({ right, model }: { right?: React.ReactNode; model?: React.ReactNode }) {
  return (
    <div className="p-4 min-h-14 border-b border-border/40 flex items-center justify-between gap-2 flex-none bg-card/50">
      <h3 className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest flex items-center gap-2 shrink-0">
        <Sparkles size={14} className="text-primary" /> AI Assistant
      </h3>
      <div className="flex items-center gap-1 min-w-0">
        {model}
        {right}
      </div>
    </div>
  )
}

function MessageRow({ message, projectId }: { message: ChatMessage; projectId: string }) {
  if (message.role === 'user') {
    const text = message.parts.map((p) => p.text).join('')
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-2xl bg-primary/10 border border-primary/10 px-3 py-2 text-[12px] whitespace-pre-wrap">
          {text}
        </div>
      </div>
    )
  }
  return (
    <div className="space-y-2">
      {message.parts.map((p) => (
        <PartView key={p.id} part={p} projectId={projectId} />
      ))}
      {message.runId && <RunEvidence runId={message.runId} />}
      {message.status === 'streaming' && (
        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <Loader2 size={11} className="animate-spin" /> 正在生成
        </div>
      )}
    </div>
  )
}

function RunEvidence({ runId }: { runId: string }) {
  const evidence = useQuery({
    queryKey: ['agent-run-evidence', runId],
    queryFn: () => agentApi.listRunEvidence(runId),
  })
  if (evidence.isLoading) {
    return <p className="text-[11px] text-muted-foreground">正在读取本次回答使用的依据…</p>
  }
  if (evidence.isError) {
    return (
      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
        <span>依据暂时无法读取</span>
        <button type="button" className="underline underline-offset-2 hover:text-foreground" onClick={() => void evidence.refetch()}>
          重试
        </button>
      </div>
    )
  }
  const rows = evidence.data ?? []
  if (rows.length === 0) return null
  return (
    <details className="rounded-xl border border-border/40 bg-muted/15 text-[11px]">
      <summary className="cursor-pointer list-none px-3 py-2 font-medium text-muted-foreground hover:text-foreground">
        参考材料 · {rows.length}
      </summary>
      <div className="space-y-2 border-t border-border/40 p-3">
        {rows.map((row) => (
          <article key={row.id} className="min-w-0 rounded-lg bg-background/70 p-2.5">
            <h4 className="break-words text-[11px] font-medium">{row.sourceName || '项目资料'}</h4>
            {row.quote && <blockquote className="mt-2 whitespace-pre-wrap break-words border-l-2 border-primary/30 pl-2.5 leading-relaxed">{row.quote}</blockquote>}
          </article>
        ))}
      </div>
    </details>
  )
}

function PartView({ part, projectId }: { part: ChatPart; projectId: string }) {
  if (part.type === 'text') {
    if (!part.text) return null
    return <div className="text-[12px] leading-relaxed whitespace-pre-wrap">{part.text}</div>
  }
  if (part.type === 'tool') return <ToolCard part={part} />
  if (part.type === 'activity') return <ActivityRow part={part} />
  if (part.type === 'proposal') {
    const pid = String((part.meta as { proposalId?: string })?.proposalId ?? '')
    return <ProposalCard proposalId={pid} projectId={projectId} />
  }
  if (part.type === 'error') return <p className="text-[11px] text-destructive">{part.text}</p>
  return null
}

function ToolCard({ part }: { part: ChatPart }) {
  const [open, setOpen] = useState(false)
  const meta = (part.meta ?? {}) as { toolName?: string; input?: unknown; output?: unknown }
  return (
    <div className="rounded-xl border border-border/40 bg-muted/20 overflow-hidden">
      <button
        className="w-full flex items-center gap-1.5 px-3 py-2 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span className="font-medium">{meta.toolName ?? '工具'}</span>
        <span className="ml-auto text-[10px] opacity-70">{part.status}</span>
      </button>
      {open && (
        <div className="px-3 pb-3 space-y-2">
          {meta.input != null && (
            <pre className="text-[10px] bg-background/60 rounded-lg p-2 overflow-x-auto">
              {JSON.stringify(meta.input, null, 2)}
            </pre>
          )}
          {meta.output != null && (
            <pre className="text-[10px] bg-background/60 rounded-lg p-2 overflow-x-auto whitespace-pre-wrap">
              {typeof meta.output === 'string' ? meta.output : JSON.stringify(meta.output, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}

/** 子 agent 活动：嵌在当前 Assistant 消息里，不另起一个发言者。 */
function ActivityRow({ part }: { part: ChatPart }) {
  const meta = (part.meta ?? {}) as { title?: string; status?: string }
  return (
    <div className="flex items-center gap-2 text-[11px] text-muted-foreground pl-1">
      {part.status === 'running' ? (
        <Loader2 size={11} className="animate-spin shrink-0" />
      ) : (
        <span className="w-[11px] text-center shrink-0">✓</span>
      )}
      <span>{meta.title ?? '子任务'}</span>
    </div>
  )
}

function ProposalCard({ proposalId, projectId }: { proposalId: string; projectId: string }) {
  const proposal = useChatStore((s) => s.proposals[proposalId])
  const decide = useChatStore((s) => s.decideProposal)
  const queryClient = useQueryClient()
  const [busy, setBusy] = useState(false)
  if (!proposal) return null

  const pending = proposal.status === 'pending'
  const label = {
    pending: '待确认',
    applied: '已应用',
    rejected: '已拒绝',
    stale: '已过期（稿件已变化）',
  }[proposal.status]

  return (
    <div className="rounded-2xl border border-border/40 bg-card overflow-hidden">
      <div className="px-3 py-2 border-b border-border/40 flex items-center gap-2">
        <span className="text-[11px] font-bold uppercase tracking-widest text-muted-foreground">修改建议</span>
        <span className="text-[10px] text-muted-foreground truncate">{proposal.filePath}</span>
        <span
          className={cn(
            'ml-auto text-[10px]',
            pending ? 'text-primary' : proposal.status === 'applied' ? 'text-green-600' : 'text-muted-foreground',
          )}
        >
          {label}
        </span>
      </div>
      <div className="px-3 py-2 space-y-1.5">
        <pre className="text-[11px] rounded-lg bg-destructive/5 text-destructive/90 p-2 overflow-x-auto whitespace-pre-wrap">
          {proposal.oldText}
        </pre>
        <pre className="text-[11px] rounded-lg bg-green-500/5 text-green-700 p-2 overflow-x-auto whitespace-pre-wrap">
          {proposal.newText}
        </pre>
        {proposal.reason && <p className="text-[11px] text-muted-foreground">{proposal.reason}</p>}
      </div>
      {pending && (
        <div className="px-3 pb-3 flex gap-2">
          <Button
            size="sm"
            className="h-7 rounded-lg text-[11px]"
            disabled={busy}
            onClick={async () => {
              setBusy(true)
              try {
                await decide(proposalId, true)
                // 联动：稿件已被服务端改写，刷新文件树与编辑器内容，
                // 否则用户会在旧内容上继续编辑、再自动保存把改动盖回去。
                await queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
                await queryClient.invalidateQueries({
                  queryKey: latexKeys.content(projectId, proposal.filePath),
                })
              } finally {
                setBusy(false)
              }
            }}
          >
            应用
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 rounded-lg text-[11px]"
            disabled={busy}
            onClick={async () => {
              setBusy(true)
              try {
                await decide(proposalId, false)
              } finally {
                setBusy(false)
              }
            }}
          >
            拒绝
          </Button>
        </div>
      )}
    </div>
  )
}

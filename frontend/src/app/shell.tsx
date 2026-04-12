import { zodResolver } from '@hookform/resolvers/zod'
import {
  AlertTriangle,
  ArrowRight,
  Blocks,
  FolderGit2,
  Layers3,
  LayoutPanelTop,
  ListChecks,
  Menu,
  Palette,
  PanelsTopLeft,
  Sparkles,
  SwatchBook,
  Waypoints,
} from 'lucide-react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { getPlannedRouteGroups } from '@/app/router'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { EmptyState, ErrorState, LoadingState } from '@/components/shared'

const foundationFormSchema = z.object({
  workspaceName: z.string().min(3, '请输入至少 3 个字符'),
  intent: z.string().min(12, '描述一下迁移目标，至少 12 个字符'),
})

type FoundationFormValues = z.infer<typeof foundationFormSchema>

const migrationPhases = [
  {
    title: 'Foundation',
    summary: 'Token、共享状态组件、Provider 与目录骨架在这一层收口。',
  },
  {
    title: 'Workflow',
    summary: '下一阶段直接接入路由壳、认证与项目列表，不再返工样式基线。',
  },
  {
    title: 'Research Space',
    summary: '上传、PDF 阅读、高亮与工作区模块在统一视觉语言上叠加。',
  },
]

const principles = [
  {
    icon: Palette,
    title: 'Intentional Visual Language',
    description:
      '颜色、字号、圆角、阴影和布局节奏集中在全局 token 里，后续页面直接复用。',
  },
  {
    icon: Blocks,
    title: 'Composable Architecture',
    description:
      '共享组件、应用层和 domain feature 已拆开，Phase 2 只需要往约定位置填业务。',
  },
  {
    icon: Waypoints,
    title: 'Verification First',
    description:
      '构建、类型检查和实际页面渲染都作为验收证据，不靠“应该没问题”过关。',
  },
]

const featureDomains = [
  {
    domain: 'auth',
    path: 'src/features/auth',
    nextStep: 'login / register schema 与 mutation',
  },
  {
    domain: 'projects',
    path: 'src/features/projects',
    nextStep: 'documents 页面、分组与创建项目流程',
  },
  {
    domain: 'trash',
    path: 'src/features/trash',
    nextStep: '回收站查询与恢复 / 删除动作',
  },
  {
    domain: 'workspace',
    path: 'src/features/workspace',
    nextStep: '项目壳层与顶部 tab 导航',
  },
  {
    domain: 'uploading',
    path: 'src/features/uploading',
    nextStep: '文件树、表格、预览、批量操作',
  },
  {
    domain: 'coding',
    path: 'src/features/coding',
    nextStep: '打开文件 tab、PDF viewer、高亮侧栏',
  },
] as const

const foundationComponents = [
  'button',
  'card',
  'dialog',
  'form',
  'input',
  'textarea',
  'tabs',
  'table',
  'sheet',
  'dropdown-menu',
  'tooltip',
  'alert',
  'badge',
  'skeleton',
  'separator',
  'scroll-area',
  'sonner',
] as const

const visualRules = [
  'Hero / display / title / body 已形成统一字号层级',
  '桌面端两栏信息密度与移动端堆叠布局共用同一套间距规则',
  'loading / empty / error 已抽成共享状态组件，不再每页重复造样式',
]

export function AppShell() {
  const routeGroups = getPlannedRouteGroups()
  const form = useForm<FoundationFormValues>({
    resolver: zodResolver(foundationFormSchema),
    defaultValues: {
      workspaceName: 'PaperSage',
      intent: '先把 Phase 1 做成可直接进入 Phase 2 的稳定底座',
    },
  })

  const handlePreviewToast = () => {
    toast.success('Foundation preview 已接通', {
      description: 'Toaster、Tooltip、shadcn 组件与全局 token 均已在当前页面验证。',
    })
  }

  const handleFoundationSubmit = form.handleSubmit((values) => {
    toast.success('表单基座可进入 Phase 2', {
      description: `${values.workspaceName} 的共享表单样式和校验链路已经就位。`,
    })
  })

  return (
    <main className="min-h-screen text-foreground">
      <div className="page-shell mx-auto flex min-h-screen w-full flex-col py-8 lg:py-10">
        <section className="surface-card relative overflow-hidden rounded-[2rem] border border-white/60 bg-[color:var(--surface-subtle)] px-6 py-6 backdrop-blur xl:px-8">
          <div className="absolute inset-x-10 top-0 h-px bg-gradient-to-r from-transparent via-amber-500/50 to-transparent" />
          <div className="absolute inset-y-0 right-0 hidden w-[34%] rounded-l-[2rem] bg-[color:var(--surface-tint)] blur-3xl lg:block" />
          <div className="relative grid gap-8 lg:grid-cols-[1.2fr_0.8fr] lg:items-end">
            <div className="flex flex-col gap-8">
              <div className="flex flex-wrap items-center gap-3">
                <div className="inline-flex items-center gap-2 rounded-full border border-amber-500/25 bg-amber-100/85 px-3 py-1 text-xs font-medium uppercase tracking-[0.24em] text-amber-950">
                  <Layers3 className="size-3.5" />
                  Phase 1 Foundation
                </div>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Badge variant="outline" className="rounded-full bg-white/70 px-3 py-1 text-[0.72rem] uppercase tracking-[0.2em]">
                      Phase 2 Ready
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent sideOffset={8}>
                    视觉基线、目录骨架、共享状态组件与基础 UI 已在本页集中验证。
                  </TooltipContent>
                </Tooltip>
              </div>

              <div className="flex flex-col gap-4">
                <p className="text-sm font-medium uppercase tracking-[0.28em] text-muted-foreground">
                  PaperSage Frontend Migration
                </p>
                <h1 className="max-w-4xl text-hero font-semibold text-slate-950">
                  A steadier React foundation for the research workspace.
                </h1>
                <p className="max-w-2xl text-base leading-8 text-muted-foreground sm:text-lg">
                  当前首页不再只是“starter 被换掉了”，而是作为前端基座预览页，直接承载 token、
                  表单、状态组件、domain 目录和迁移路线，为 Phase 2 的路由与基础设施接入做准备。
                </p>
              </div>

              <div className="flex flex-wrap gap-3">
                <Button size="lg" className="rounded-full px-4" onClick={handlePreviewToast}>
                  <Sparkles data-icon="inline-start" />
                  预览全局反馈
                  <ArrowRight data-icon="inline-end" />
                </Button>
                <Sheet>
                  <SheetTrigger asChild>
                    <Button variant="outline" size="lg" className="rounded-full bg-white/70 px-4">
                      <Menu data-icon="inline-start" />
                      查看移动端基线
                    </Button>
                  </SheetTrigger>
                  <SheetContent className="w-full sm:max-w-md">
                    <SheetHeader>
                      <SheetTitle>移动端布局基线</SheetTitle>
                      <SheetDescription>
                        Phase 1 已约定移动端使用单列堆叠，顶部摘要优先，次级信息收进 sheet / tabs。
                      </SheetDescription>
                    </SheetHeader>
                    <div className="flex flex-col gap-4 px-4 pb-4">
                      {visualRules.map((rule) => (
                        <Card key={rule} size="sm" className="border border-border/70 bg-background/90">
                          <CardContent className="px-4 py-3 text-sm leading-6 text-muted-foreground">
                            {rule}
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  </SheetContent>
                </Sheet>
              </div>
            </div>

            <Card className="surface-strong rounded-[1.75rem] border border-white/10 bg-[color:var(--surface-strong)] text-white">
              <CardHeader className="gap-3">
                <div className="flex items-center justify-between gap-4">
                  <CardTitle className="text-base text-white">Foundation Stack</CardTitle>
                  <Badge variant="secondary" className="rounded-full bg-white/10 text-white">
                    pnpm
                  </Badge>
                </div>
                <CardDescription className="text-slate-300">
                  当前技术栈已经覆盖路由、状态、表单、图标、样式系统与 PDF 预览基础依赖。
                </CardDescription>
              </CardHeader>
              <CardContent className="grid gap-3">
                {[
                  'Vite + React + TypeScript',
                  'TanStack Query / Router',
                  'Tailwind + shadcn/ui + Radix',
                  'Zustand + RHF + Zod + Lucide + PDF.js',
                ].map((item) => (
                  <div
                    key={item}
                    className="flex items-center justify-between rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm text-slate-200"
                  >
                    <span>{item}</span>
                    <ArrowRight className="size-4 text-amber-300" />
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        </section>

        <section className="mt-8 grid gap-5 lg:grid-cols-3">
          {principles.map(({ icon: Icon, title, description }) => (
            <Card
              key={title}
              className="surface-card rounded-[1.75rem] border border-border/70 bg-white/85 p-2"
            >
              <CardHeader className="px-4 pt-4">
                <div className="mb-3 inline-flex rounded-2xl bg-primary p-3 text-accent">
                  <Icon className="size-5" />
                </div>
                <CardTitle className="text-xl tracking-[-0.04em] text-slate-950">{title}</CardTitle>
              </CardHeader>
              <CardContent className="px-4 pb-4 pt-0">
                <CardDescription className="leading-7 text-muted-foreground">{description}</CardDescription>
              </CardContent>
            </Card>
          ))}
        </section>

        <section className="mt-8 grid gap-6 lg:grid-cols-[1.1fr_0.9fr]">
          <Card className="surface-card rounded-[1.85rem] border border-border/70 bg-white/88 p-2">
            <CardHeader className="px-4 pt-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-3 text-sm font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  <SwatchBook className="size-4 text-amber-700" />
                  Foundation Preview
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="outline" size="sm" className="rounded-full bg-background">
                      <ListChecks data-icon="inline-start" />
                      查看覆盖面
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuLabel>Phase 1 基线</DropdownMenuLabel>
                    <DropdownMenuGroup>
                      <DropdownMenuItem onSelect={handlePreviewToast}>
                        触发 toast 验证
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          toast.message('路由骨架已定义', {
                            description: `${routeGroups.main.length + routeGroups.project.length + routeGroups.public.length} 条规划路径可供 Phase 2 继续接入。`,
                          })
                        }
                      >
                        查看路由规划
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onSelect={() =>
                        toast.message('共享状态组件已就位', {
                          description: 'loading / empty / error 可直接复用于后续页面。',
                        })
                      }
                    >
                      查看页面状态基线
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </CardHeader>
            <CardContent className="px-4 pb-4">
              <Tabs defaultValue="baseline" className="gap-5">
                <TabsList className="w-full justify-start overflow-x-auto rounded-2xl bg-muted/80 p-1">
                  <TabsTrigger value="baseline">Design Tokens</TabsTrigger>
                  <TabsTrigger value="states">Page States</TabsTrigger>
                  <TabsTrigger value="readiness">Domain Skeleton</TabsTrigger>
                </TabsList>

                <TabsContent value="baseline" className="grid gap-5 lg:grid-cols-[0.95fr_1.05fr]">
                  <Card className="border border-border/70 bg-[#fffdf9] panel-grid">
                    <CardHeader>
                      <CardTitle className="text-title">视觉规则已经集中管理</CardTitle>
                      <CardDescription>
                        色板、字体、半径、阴影和布局节奏都沉淀到全局样式入口，后续业务页不再单独发明规范。
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-4">
                      <div className="grid gap-3 sm:grid-cols-2">
                        {[
                          ['Canvas', 'bg-background text-foreground'],
                          ['Panel', 'bg-card text-card-foreground'],
                          ['Accent', 'bg-accent text-accent-foreground'],
                          ['Signal', 'bg-primary text-primary-foreground'],
                        ].map(([label, className]) => (
                          <div key={label} className={`rounded-2xl border border-border/70 px-4 py-4 ${className}`}>
                            <p className="text-xs uppercase tracking-[0.22em] opacity-70">{label}</p>
                            <p className="mt-2 font-medium">{label} token</p>
                          </div>
                        ))}
                      </div>

                      <FieldGroup>
                        {visualRules.map((rule) => (
                          <Field key={rule}>
                            <FieldLabel>{rule}</FieldLabel>
                            <FieldDescription>
                              这条规则已经进入共享设计语言，后续页面沿用，不在业务组件里重复描述。
                            </FieldDescription>
                          </Field>
                        ))}
                      </FieldGroup>
                    </CardContent>
                  </Card>

                  <Card className="border border-border/70 bg-white/92">
                    <CardHeader>
                      <CardTitle className="text-title">表单基础件已可复用</CardTitle>
                      <CardDescription>
                        React Hook Form、Zod、Field / Form primitives 已接通，用作 Auth 和项目创建对话框的基座。
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Form {...form}>
                        <form className="flex flex-col gap-4" onSubmit={handleFoundationSubmit}>
                          <FormField
                            control={form.control}
                            name="workspaceName"
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>Workspace Name</FormLabel>
                                <FormControl>
                                  <Input placeholder="PaperSage" {...field} />
                                </FormControl>
                                <FormDescription>作为 Phase 2 业务壳层的命名占位。</FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                          <FormField
                            control={form.control}
                            name="intent"
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>Migration Intent</FormLabel>
                                <FormControl>
                                  <Textarea
                                    placeholder="把 foundation 做成可直接接业务的稳定底座"
                                    {...field}
                                  />
                                </FormControl>
                                <FormDescription>统一的多行输入样式和校验提示已经可直接使用。</FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                          <div className="flex flex-wrap gap-3">
                            <Button type="submit">
                              <Sparkles data-icon="inline-start" />
                              验证表单链路
                            </Button>
                            <Button
                              type="button"
                              variant="outline"
                              onClick={() => form.reset()}
                            >
                              重置
                            </Button>
                          </div>
                        </form>
                      </Form>
                    </CardContent>
                  </Card>
                </TabsContent>

                <TabsContent value="states" className="grid gap-5 lg:grid-cols-3">
                  <LoadingState
                    title="Loading State"
                    description="统一显示轻量骨架屏和操作上下文，不再用裸 `animate-pulse` 临时拼装。"
                  />
                  <EmptyState
                    title="Empty State"
                    description="项目列表、回收站、上传列表都可以复用同一套留白和行动按钮风格。"
                    action={
                      <Button variant="outline" size="sm" onClick={handlePreviewToast}>
                        查看反馈样式
                      </Button>
                    }
                  />
                  <ErrorState
                    title="Error State"
                    description="错误状态统一用 alert 结构和恢复动作，避免每个页面单独决定危险色和文案布局。"
                    details="示例：后续 API client 归一化错误后，可直接把错误内容注入到这个组件。"
                    action={
                      <Button size="sm" onClick={handlePreviewToast}>
                        模拟重试
                      </Button>
                    }
                  />
                </TabsContent>

                <TabsContent value="readiness" className="grid gap-5 lg:grid-cols-[1.05fr_0.95fr]">
                  <Card className="border border-border/70 bg-white/92">
                    <CardHeader>
                      <CardTitle className="text-title">Domain Feature Skeleton</CardTitle>
                      <CardDescription>
                        每个业务域已经有独立目录作为落点，Phase 2 直接在既定边界内补 API、hooks 与组件。
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Domain</TableHead>
                            <TableHead>Path</TableHead>
                            <TableHead>Next Step</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {featureDomains.map((item) => (
                            <TableRow key={item.domain}>
                              <TableCell>
                                <Badge variant="outline" className="rounded-full">
                                  {item.domain}
                                </Badge>
                              </TableCell>
                              <TableCell className="font-mono text-xs text-muted-foreground">
                                {item.path}
                              </TableCell>
                              <TableCell className="text-muted-foreground">{item.nextStep}</TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </CardContent>
                  </Card>

                  <Card className="border border-border/70 bg-white/92">
                    <CardHeader>
                      <CardTitle className="text-title">Route & Component Readiness</CardTitle>
                      <CardDescription>
                        路由规划、首批 shadcn 组件和迁移顺序在一个地方汇总，可直接作为 Phase 2 施工清单。
                      </CardDescription>
                      <CardAction>
                        <Badge variant="secondary" className="rounded-full">
                          {foundationComponents.length} components
                        </Badge>
                      </CardAction>
                    </CardHeader>
                    <CardContent className="grid gap-4">
                      <ScrollArea className="h-52 rounded-2xl border border-border/70 bg-muted/30">
                        <div className="grid gap-2 p-3">
                          {foundationComponents.map((component) => (
                            <div
                              key={component}
                              className="flex items-center justify-between rounded-xl border border-border/70 bg-background/80 px-3 py-2"
                            >
                              <span className="font-mono text-xs">{component}</span>
                              <Badge variant="outline" className="rounded-full">
                                ready
                              </Badge>
                            </div>
                          ))}
                        </div>
                      </ScrollArea>

                      <div className="grid gap-3">
                        {Object.entries(routeGroups).map(([group, paths]) => (
                          <div key={group} className="rounded-2xl border border-border/70 bg-muted/30 px-4 py-3">
                            <div className="flex items-center justify-between gap-3">
                              <div className="flex items-center gap-2">
                                <FolderGit2 className="size-4 text-amber-700" />
                                <span className="font-medium capitalize">{group}</span>
                              </div>
                              <Badge variant="outline" className="rounded-full">
                                {paths.length}
                              </Badge>
                            </div>
                            <p className="mt-2 text-sm leading-6 text-muted-foreground">
                              {paths.join(' · ')}
                            </p>
                          </div>
                        ))}
                      </div>
                    </CardContent>
                  </Card>
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

          <div className="grid gap-6">
            <Card className="surface-card rounded-[1.85rem] border border-border/70 bg-white/88 p-2">
              <CardHeader className="px-4 pt-4">
                <div className="flex items-center gap-3 text-sm font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  <PanelsTopLeft className="size-4 text-amber-700" />
                  Phase Map
                </div>
              </CardHeader>
              <CardContent className="grid gap-4 px-4 pb-4">
                {migrationPhases.map((phase, index) => (
                  <div key={phase.title} className="rounded-[1.4rem] border border-border/70 bg-background/85 px-5 py-4">
                    <div className="flex items-center justify-between gap-4">
                      <h3 className="text-lg font-semibold tracking-[-0.04em] text-slate-950">
                        {phase.title}
                      </h3>
                      <Badge variant="secondary" className="rounded-full">
                        0{index + 1}
                      </Badge>
                    </div>
                    <p className="mt-2 text-sm leading-7 text-muted-foreground">{phase.summary}</p>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card className="surface-card rounded-[1.85rem] border border-border/70 bg-white/88 p-2">
              <CardHeader className="px-4 pt-4">
                <div className="flex items-center gap-3 text-sm font-medium uppercase tracking-[0.24em] text-muted-foreground">
                  <LayoutPanelTop className="size-4 text-amber-700" />
                  Foundation Summary
                </div>
              </CardHeader>
              <CardContent className="grid gap-4 px-4 pb-4">
                <Alert>
                  <AlertTriangle className="size-4" />
                  <AlertTitle>Phase 1 目标已重新定义为“能直接接业务”</AlertTitle>
                  <AlertDescription>
                    当前基线覆盖视觉 token、统一状态组件、完整基础 UI 和 domain 骨架，下一步可以直接进入 env / API client / store / provider。
                  </AlertDescription>
                </Alert>
                <Separator />
                <FieldGroup>
                  <Field>
                    <FieldLabel>Typography</FieldLabel>
                    <FieldDescription>Hero / display / title / body 字级和字距已集中在全局 token。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel>Spacing & Radius</FieldLabel>
                    <FieldDescription>页边距、section gap、圆角与投影统一，桌面与移动端共享同一套尺度。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel>Reusable States</FieldLabel>
                    <FieldDescription>loading / empty / error 已抽成共享组件，可直接挂到后续路由页面。</FieldDescription>
                  </Field>
                </FieldGroup>
              </CardContent>
              <CardFooter className="justify-between gap-3">
                <Badge variant="outline" className="rounded-full">
                  Ready for Phase 2
                </Badge>
                <Button variant="outline" size="sm" onClick={handlePreviewToast}>
                  <Palette data-icon="inline-start" />
                  再次验证反馈
                </Button>
              </CardFooter>
            </Card>
          </div>
        </section>
      </div>
    </main>
  )
}

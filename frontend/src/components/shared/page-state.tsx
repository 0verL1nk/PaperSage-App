import type { ReactNode } from 'react'
import { Inbox, LoaderCircle, Sparkles, TriangleAlert, type LucideIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/cn'

type PageStateProps = {
  title: string
  description: string
  icon?: LucideIcon
  action?: ReactNode
  children?: ReactNode
  className?: string
}

function PageState({
  title,
  description,
  icon: Icon = Sparkles,
  action,
  children,
  className,
}: PageStateProps) {
  return (
    <Card className={cn('surface-card h-full rounded-[1.6rem] border border-border/70 bg-white/92', className)}>
      <CardHeader className="gap-4">
        <div className="inline-flex size-11 items-center justify-center rounded-2xl bg-muted text-primary">
          <Icon className="size-5" />
        </div>
        <div className="flex flex-col gap-2">
          <CardTitle className="text-title">{title}</CardTitle>
          <p className="text-sm leading-7 text-muted-foreground">{description}</p>
        </div>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-4">
        {children}
        {action ? <div className="pt-1">{action}</div> : null}
      </CardContent>
    </Card>
  )
}

type LoadingStateProps = Omit<PageStateProps, 'children' | 'icon'>

function LoadingState(props: LoadingStateProps) {
  return (
    <PageState icon={LoaderCircle} {...props}>
      <div className="grid gap-3">
        <Skeleton className="h-4 w-2/5 rounded-full" />
        <Skeleton className="h-20 w-full rounded-3xl" />
        <Skeleton className="h-4 w-4/5 rounded-full" />
        <Skeleton className="h-4 w-3/5 rounded-full" />
      </div>
    </PageState>
  )
}

type EmptyStateProps = Omit<PageStateProps, 'icon'>

function EmptyState(props: EmptyStateProps) {
  return <PageState icon={Inbox} {...props} />
}

type ErrorStateProps = Omit<PageStateProps, 'icon' | 'children'> & {
  details?: string
}

function ErrorState({ details, ...props }: ErrorStateProps) {
  return (
    <PageState icon={TriangleAlert} {...props}>
      {details ? (
        <Alert variant="destructive">
          <TriangleAlert className="size-4" />
          <AlertTitle>Recoverable error</AlertTitle>
          <AlertDescription>{details}</AlertDescription>
        </Alert>
      ) : null}
    </PageState>
  )
}

export { EmptyState, ErrorState, LoadingState, PageState }

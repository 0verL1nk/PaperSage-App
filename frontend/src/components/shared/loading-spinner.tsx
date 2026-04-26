import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'

interface LoadingSpinnerProps {
  text?: string
  progress?: number // 0 to 100
  className?: string
  iconClassName?: string
}

export function LoadingSpinner({ text, progress, className, iconClassName }: LoadingSpinnerProps) {
  return (
    <div className={cn("flex flex-col items-center justify-center p-12 gap-4", className)}>
      <div className="relative flex items-center justify-center">
        <Loader2 className={cn("h-10 w-10 animate-spin text-primary/40", iconClassName)} />
        {progress !== undefined && progress > 0 && (
          <span className="absolute text-[10px] font-bold text-primary/60 tabular-nums">
            {Math.round(progress)}%
          </span>
        )}
      </div>
      {text && (
        <p className="text-xs text-muted-foreground font-medium uppercase tracking-widest">
          {text} {progress !== undefined && progress > 0 ? `(${Math.round(progress)}%)` : ''}
        </p>
      )}
    </div>
  )
}

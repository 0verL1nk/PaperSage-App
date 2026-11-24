import { FileText, Play } from 'lucide-react'

import { Button } from '@/components/ui/button'

/** 未编译时的预览空态：一个真正的「编译当前项目」按钮，而不是看得见点不动的图标。 */
export function EmptyPreview({ onCompile, disabled }: { onCompile: () => void; disabled: boolean }) {
  return (
    <div className="mx-auto flex min-h-[480px] w-full max-w-[420px] flex-col items-center justify-center rounded-lg border border-border/60 bg-white p-8 text-center shadow-sm">
      <div className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
        <FileText className="size-6" />
      </div>
      <h4 className="mt-4 text-sm font-semibold">还没有生成预览</h4>
      <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
        点下面的按钮编译当前项目。第一次编译会先准备浏览器内的 LaTeX 环境，可能需要几十秒。
      </p>
      <Button size="sm" className="mt-5 h-8 gap-1.5" onClick={onCompile} disabled={disabled}>
        <Play size={13} className="fill-current" />
        编译当前项目
      </Button>
    </div>
  )
}

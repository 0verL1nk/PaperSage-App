import { FileText, Play } from 'lucide-react'

import { StateBlock } from '@/components/shared'
import { Button } from '@/components/ui/button'

/** 未编译时的预览空态：一个真正的「编译当前项目」按钮，而不是看得见点不动的图标。 */
export function EmptyPreview({ onCompile, disabled }: { onCompile: () => void; disabled: boolean }) {
  return (
    <StateBlock
      title="还没有生成预览"
      description="点下面的按钮编译当前项目。第一次编译需要在浏览器内准备 LaTeX 环境，并按需下载宏包与字体，通常需要 1–3 分钟；完成后再编译通常几秒即可。"
      icon={FileText}
      className="mx-auto min-h-[480px] w-full max-w-[420px] justify-center"
      action={
        <Button size="sm" className="h-8 gap-1.5" onClick={onCompile} disabled={disabled}>
          <Play size={13} className="fill-current" />
          编译当前项目
        </Button>
      }
    />
  )
}

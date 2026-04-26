/** Floating batch actions bar shown when documents are selected. */

import { FolderInput, Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface BatchActionsBarProps {
  selectedCount: number
  onBatchMove: () => void
  onBatchRemark: () => void
  onBatchDelete: () => void
  onClearSelection: () => void
}

export function BatchActionsBar({
  selectedCount, onBatchMove, onBatchRemark, onBatchDelete, onClearSelection,
}: BatchActionsBarProps) {
  if (selectedCount === 0) return null

  return (
    <div className="flex items-center gap-3 px-5 py-2 bg-accent/5 border-t border-accent/10">
      <span className="text-[11px] font-medium text-accent-foreground">
        已选 {selectedCount} 项
      </span>
      <div className="flex items-center gap-1.5">
        <Button variant="outline" size="sm" className="h-7 text-[11px] gap-1 rounded-full border-border/40" onClick={onBatchMove}>
          <FolderInput size={13} /> 移动
        </Button>
        <Button variant="outline" size="sm" className="h-7 text-[11px] gap-1 rounded-full border-border/40" onClick={onBatchRemark}>
          <Pencil size={13} /> 备注
        </Button>
        <Button variant="outline" size="sm" className="h-7 text-[11px] gap-1 rounded-full border-border/40 text-destructive hover:text-destructive" onClick={onBatchDelete}>
          <Trash2 size={13} /> 删除
        </Button>
      </div>
      <Button variant="ghost" size="sm" className="h-7 text-[11px] ml-auto rounded-full" onClick={onClearSelection}>
        取消选择
      </Button>
    </div>
  )
}

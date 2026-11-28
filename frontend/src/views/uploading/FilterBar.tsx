/** Filter bar with pill type tabs, folder selector, and search. */

import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { FILE_TYPE_LABELS } from './constants'

interface FilterBarProps {
  typeFilter: string
  onTypeFilterChange: (value: string) => void
  folderId: string | null
  folders: { id: string; name: string }[]
  onFolderChange: (id: string | null) => void
  searchQuery: string
  onSearchChange: (query: string) => void
}

export function FilterBar({
  typeFilter, onTypeFilterChange,
  folderId, folders, onFolderChange,
  searchQuery, onSearchChange,
}: FilterBarProps) {
  return (
    <div className="flex flex-col gap-2 border-b border-border/30 px-4 py-2.5 sm:px-5">
      {/* 类型胶囊：单行横滚，不换行堆成三排 */}
      <div className="flex items-center gap-1 overflow-x-auto no-scrollbar">
        {FILE_TYPE_LABELS.map((ft) => (
          <Button
            key={ft.value}
            variant="ghost"
            size="sm"
            className={cn(
              'h-11 shrink-0 rounded-full px-3 text-[11px] font-medium transition-all sm:h-7',
              typeFilter === ft.value
                ? 'bg-background shadow-sm ring-1 ring-border/50 text-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
            onClick={() => onTypeFilterChange(ft.value)}
          >
            {ft.label}
          </Button>
        ))}
      </div>

      {/* 文件夹 + 搜索固定一行 */}
      <div className="flex items-center gap-2">
        <Select
          value={folderId ?? '_all'}
          onValueChange={(v) => onFolderChange(v === '_all' ? null : v)}
        >
          <SelectTrigger className="h-11 w-[132px] shrink-0 rounded-lg border-border/50 text-xs sm:h-8 sm:w-[180px]">
            <SelectValue placeholder="全部文件夹" />
          </SelectTrigger>
          <SelectContent className="rounded-lg">
            <SelectItem value="_all" className="text-xs">全部文件夹</SelectItem>
            {folders.map((f) => (
              <SelectItem key={f.id} value={f.id} className="text-xs">{f.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="relative min-w-0 flex-1 sm:max-w-[260px]">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground/60" />
          <Input
            placeholder="搜索文档..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            className="h-11 rounded-xl border-border/40 bg-muted/30 pl-9 text-xs focus-visible:ring-accent/30 sm:h-8"
          />
        </div>
      </div>
    </div>
  )
}

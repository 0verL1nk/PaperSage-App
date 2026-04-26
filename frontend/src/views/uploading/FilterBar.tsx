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
    <div className="flex flex-col gap-3 px-5 py-3 border-b border-border/30">
      <div className="flex items-center gap-1.5">
        {FILE_TYPE_LABELS.map((ft) => (
          <Button
            key={ft.value}
            variant="ghost"
            size="sm"
            className={cn(
              'h-7 text-[11px] rounded-full px-3 font-medium transition-all',
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

      <div className="flex items-center gap-3">
        <Select
          value={folderId ?? '_all'}
          onValueChange={(v) => onFolderChange(v === '_all' ? null : v)}
        >
          <SelectTrigger className="w-[180px] h-8 text-xs rounded-lg border-border/50">
            <SelectValue placeholder="全部文件夹" />
          </SelectTrigger>
          <SelectContent className="rounded-lg">
            <SelectItem value="_all" className="text-xs">全部文件夹</SelectItem>
            {folders.map((f) => (
              <SelectItem key={f.id} value={f.id} className="text-xs">{f.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="relative flex-1 max-w-[260px]">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground/60" />
          <Input
            placeholder="搜索文档..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            className="h-8 text-xs pl-9 rounded-xl bg-muted/30 border-border/40 focus-visible:ring-accent/30"
          />
        </div>
      </div>
    </div>
  )
}

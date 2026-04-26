/** Column definitions for the documents data table. */

import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { MoreHorizontal, FolderInput, Pencil, Trash2 } from 'lucide-react'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ColumnDef } from '@tanstack/react-table'
import type { ProjectDocument } from '@/types/files'
import { formatFileSize, getShortType } from './constants'

export interface DocumentActions {
  onMove: (doc: ProjectDocument) => void
  onRemark: (doc: ProjectDocument) => void
  onDelete: (doc: ProjectDocument) => void
  onPreview: (doc: ProjectDocument) => void
}

export function createColumns(actions: DocumentActions): ColumnDef<ProjectDocument>[] {
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected() || (table.getIsSomePageRowsSelected() && 'indeterminate')}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label="Select all"
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label="Select row"
        />
      ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    },
    {
      accessorKey: 'name',
      header: '名称',
      cell: ({ row }) => {
        const doc = row.original
        return (
          <button
            className="text-left font-medium truncate max-w-[300px] rounded px-1 -mx-1 py-0.5 transition-colors hover:bg-accent/10 hover:text-accent-foreground"
            onClick={() => actions.onPreview(doc)}
          >
            {doc.name}
          </button>
        )
      },
      size: 300,
    },
    {
      accessorKey: 'size',
      header: '大小',
      cell: ({ row }) => <span className="text-muted-foreground">{formatFileSize(row.original.size)}</span>,
      size: 90,
    },
    {
      accessorKey: 'type',
      header: '类型',
      cell: ({ row }) => {
        const short = getShortType(row.original.type)
        return (
          <span className="inline-flex items-center rounded-full bg-accent/10 px-2.5 py-0.5 text-[10px] font-semibold uppercase text-accent-foreground">
            {short}
          </span>
        )
      },
      size: 80,
    },
    {
      accessorKey: 'remark',
      header: '备注',
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm truncate max-w-[200px] block">
          {row.original.remark || '\u2014'}
        </span>
      ),
      size: 200,
    },
    {
      accessorKey: 'createdAt',
      header: '日期',
      cell: ({ row }) => {
        const raw = row.original.createdAt
        if (!raw) return '\u2014'
        const date = new Date(raw)
        return <span className="text-muted-foreground text-sm">{date.toLocaleDateString()}</span>
      },
      size: 100,
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => {
        const doc = row.original
        return (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-xs" className="h-7 w-7">
                <MoreHorizontal size={14} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => actions.onMove(doc)}>
                <FolderInput size={14} className="mr-2" /> 移动
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => actions.onRemark(doc)}>
                <Pencil size={14} className="mr-2" /> 备注
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => actions.onDelete(doc)} className="text-destructive">
                <Trash2 size={14} className="mr-2" /> 删除
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )
      },
      size: 50,
      enableSorting: false,
    },
  ]
}

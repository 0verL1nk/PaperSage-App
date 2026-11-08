/** Data table for documents with sorting, pagination, and row selection. */

import { useEffect, useState } from 'react'
import {
  useReactTable, getCoreRowModel, getSortedRowModel, getPaginationRowModel,
  getFilteredRowModel, flexRender,
} from '@tanstack/react-table'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { ProjectDocument } from '@/types/files'
import { createColumns, type DocumentActions } from './columns'

interface DataTableProps {
  data: ProjectDocument[]
  actions: DocumentActions
  /** 当前范围内（搜索/类型筛选前）的文档总数，用于区分空库与无匹配结果。 */
  totalCount: number
  onClearFilters: () => void
  /** Called when row selection changes with the array of selected row IDs. */
  onRowSelectionChange?: (selectedIds: string[]) => void
}

const PAGE_SIZE = 20

export function DataTable({ data, actions, totalCount, onClearFilters, onRowSelectionChange }: DataTableProps) {
  const [sorting, setSorting] = useState<{ id: string; desc: boolean }[]>([])
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({})
  const columns = createColumns(actions)

  // 行被删除/数据刷新后，清掉指向不存在文档的选中项并同步父级
  useEffect(() => {
    const live = new Set(data.map((d) => d.id))
    const stale = Object.entries(rowSelection).filter(([id, v]) => v && !live.has(id))
    if (stale.length > 0) {
      const pruned = Object.fromEntries(Object.entries(rowSelection).filter(([id, v]) => v && live.has(id)))
      setRowSelection(pruned)
      onRowSelectionChange?.(Object.keys(pruned))
    }
  }, [data]) // eslint-disable-line react-hooks/exhaustive-deps

  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Table cannot be safely memoized
  const table = useReactTable({
    data,
    columns,
    // 选中态以文档 ID 为 key（默认的行索引在删除/刷新后会让选中
    // 「漂移」到补位上来的新行——多选删除后误选后续文件的根源）
    getRowId: (row) => row.id,
    state: { sorting, rowSelection },
    onSortingChange: setSorting,
    onRowSelectionChange: (updater) => {
      const newSelection = typeof updater === 'function' ? updater(rowSelection) : updater
      setRowSelection(newSelection)
      onRowSelectionChange?.(Object.keys(newSelection).filter((k) => newSelection[k]))
    },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    initialState: { pagination: { pageSize: PAGE_SIZE } },
    enableRowSelection: true,
  })

  // 数据变化（删除/移动/刷新）后，把已不存在的行从选中态与父级剔除
  useEffect(() => {
    const dataIds = new Set(data.map((d) => d.id))
    setRowSelection((prev) => {
      const next = Object.fromEntries(
        Object.entries(prev).filter(([id, v]) => v && dataIds.has(id)),
      )
      if (Object.keys(next).length === Object.keys(prev).length) return prev
      onRowSelectionChange?.(Object.keys(next))
      return next
    })
  }, [data, onRowSelectionChange])

  return (
    <div className="flex flex-col h-full w-full">
      <div className="flex-1 overflow-y-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id}>
                {hg.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    style={{ width: header.getSize() !== 150 ? header.getSize() : undefined }}
                    className="cursor-pointer select-none text-[11px] uppercase tracking-wider text-muted-foreground"
                    onClick={header.column.getToggleSortingHandler()}
                  >
                    {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                    {{ asc: ' \u2191', desc: ' \u2193' }[header.column.getIsSorted() as string] ?? ''}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id} data-state={row.getIsSelected() && 'selected'}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id} className="py-2">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-32 text-center text-muted-foreground">
                  {totalCount === 0 ? (
                    <span>还没有资料，点击右上角「上传」添加 PDF、DOCX 或 TXT。</span>
                  ) : (
                    <div className="flex flex-col items-center gap-1.5">
                      <span>没有匹配的结果</span>
                      <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={onClearFilters}>
                        清除搜索和筛选
                      </Button>
                    </div>
                  )}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between px-4 py-3 border-t border-border/30">
        <span className="text-[11px] text-muted-foreground">
          {table.getFilteredRowModel().rows.length} 篇文档
        </span>
        <div className="flex items-center gap-1.5">
          <Button
            variant="outline" size="sm"
            className="h-7 w-7 p-0 text-xs rounded-full border-border/40"
            onClick={() => table.previousPage()} disabled={!table.getCanPreviousPage()}
          >
            <ChevronLeft size={13} />
          </Button>
          <span className="text-[11px] text-muted-foreground tabular-nums px-1">
            第 {table.getState().pagination.pageIndex + 1} / {Math.max(table.getPageCount(), 1)} 页
          </span>
          <Button
            variant="outline" size="sm"
            className="h-7 w-7 p-0 text-xs rounded-full border-border/40"
            onClick={() => table.nextPage()} disabled={!table.getCanNextPage()}
          >
            <ChevronRight size={13} />
          </Button>
        </div>
      </div>
    </div>
  )
}

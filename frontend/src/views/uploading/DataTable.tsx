/** Data table for documents with sorting, pagination, and row selection. */

import { useState } from 'react'
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
  globalFilter: string
}

const PAGE_SIZE = 20

export function DataTable({ data, actions, globalFilter }: DataTableProps) {
  const [sorting, setSorting] = useState<{ id: string; desc: boolean }[]>([])
  const columns = createColumns(actions)

  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    initialState: { pagination: { pageSize: PAGE_SIZE } },
    enableRowSelection: true,
  })

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
                  暂无文档
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

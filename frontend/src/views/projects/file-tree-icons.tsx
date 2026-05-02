/* eslint-disable react-refresh/only-export-components */
import { cn } from '@/lib/utils'
import {
  FileText,
  FileCode,
  FileImage,
  File,
  BookOpen,
} from 'lucide-react'

/** File type → icon + color mapping for LaTeX project files. */
export interface FileTypeIconDef {
  icon: React.ComponentType<{ size?: number; className?: string }>
  className: string
}

const FILE_TYPE_MAP: Record<string, FileTypeIconDef> = {
  '.tex': { icon: FileText, className: 'text-primary' },
  '.bib': { icon: BookOpen, className: 'text-amber-500' },
  '.cls': { icon: FileCode, className: 'text-purple-500' },
  '.sty': { icon: FileCode, className: 'text-indigo-500' },
  '.bst': { icon: FileCode, className: 'text-pink-500' },
  '.tikz': { icon: FileCode, className: 'text-cyan-500' },
  '.png': { icon: FileImage, className: 'text-green-500' },
  '.jpg': { icon: FileImage, className: 'text-green-500' },
  '.jpeg': { icon: FileImage, className: 'text-green-500' },
  '.eps': { icon: FileImage, className: 'text-green-500' },
  '.pdf': { icon: FileText, className: 'text-red-500' },
}

const DEFAULT_ICON: FileTypeIconDef = { icon: File, className: 'text-muted-foreground' }

export function getFileTypeIcon(fileName: string): FileTypeIconDef {
  const ext = fileName.includes('.') ? '.' + fileName.split('.').pop()?.toLowerCase() : ''
  return FILE_TYPE_MAP[ext] ?? DEFAULT_ICON
}

export function FileTypeIcon({ name }: { name: string }) {
  const { icon: Icon, className } = getFileTypeIcon(name)
  return <Icon size={14} className={cn('shrink-0', className)} />
}

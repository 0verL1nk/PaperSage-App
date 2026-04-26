/** Upload dropzone with file type validation and progress tracking. */

import { useCallback, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import { Upload, FileText, Loader2, CheckCircle2, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useUploadDocument } from '@/services/files-api'
import { useUploadStore } from '@/store/upload-store'
import { ACCEPTED_FILE_TYPES, MAX_FILE_SIZE_BYTES } from './constants'

interface UploadDropzoneProps {
  projectId: string
  folderId: string | null
}

interface UploadStatus {
  name: string
  status: 'uploading' | 'done' | 'error'
  error?: string
}

export function UploadDropzone({ projectId, folderId }: UploadDropzoneProps) {
  const [uploads, setUploads] = useState<UploadStatus[]>([])
  const uploadMutation = useUploadDocument(projectId, folderId)
  const { setUploading } = useUploadStore()

  const onDrop = useCallback(
    async (acceptedFiles: File[]) => {
      setUploading(true)
      const statuses = acceptedFiles.map((f) => ({ name: f.name, status: 'uploading' as const }))
      setUploads(statuses)

      for (let i = 0; i < acceptedFiles.length; i++) {
        const file = acceptedFiles[i]
        try {
          await uploadMutation.mutateAsync({
            file,
            onUploadProgress: (e) => {
              if (e.total) {
                const pct = Math.round((e.loaded / e.total) * 100)
                useUploadStore.getState().setProgress(file.name, pct)
              }
            },
          })
          setUploads((prev) => prev.map((u, j) => (j === i ? { ...u, status: 'done' } : u)))
        } catch (err) {
          setUploads((prev) =>
            prev.map((u, j) =>
              j === i ? { ...u, status: 'error', error: err instanceof Error ? err.message : 'Upload failed' } : u,
            ),
          )
        }
      }
      setUploading(false)
    },
    [uploadMutation, setUploading],
  )

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: ACCEPTED_FILE_TYPES,
    maxSize: MAX_FILE_SIZE_BYTES,
    multiple: true,
  })

  return (
    <div className="space-y-2">
      <div
        {...getRootProps()}
        className={cn(
          'border-2 border-dashed rounded-2xl p-8 text-center cursor-pointer transition-all',
          isDragActive
            ? 'border-accent bg-accent/5 scale-[1.01]'
            : 'border-accent/20 hover:border-accent/40 hover:bg-accent/5',
        )}
      >
        <input {...getInputProps()} />
        <div className="w-12 h-12 rounded-2xl bg-accent/10 flex items-center justify-center mx-auto mb-3">
          <Upload size={22} className="text-accent-foreground/70" />
        </div>
        <p className="text-sm text-muted-foreground">
          {isDragActive ? '松开以上传文件...' : '拖拽文件到此处，或点击选择'}
        </p>
        <p className="text-[10px] text-muted-foreground/50 mt-1.5">
          支持 PDF、DOCX、TXT — 最大 50 MB
        </p>
      </div>

      {uploads.length > 0 && (
        <div className="space-y-1">
          {uploads.map((u) => (
            <div key={u.name} className="flex items-center gap-2 text-xs px-3 py-1.5 rounded-xl bg-muted/30">
              <FileText size={12} className="text-muted-foreground flex-none" />
              <span className="truncate flex-1">{u.name}</span>
              {u.status === 'uploading' && <Loader2 size={12} className="animate-spin text-accent-foreground" />}
              {u.status === 'done' && <CheckCircle2 size={12} className="text-amber-600" />}
              {u.status === 'error' && <XCircle size={12} className="text-destructive" />}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

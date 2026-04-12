import { z } from 'zod'

export const folderSchema = z.object({
  id: z.string(),
  name: z.string(),
  parentId: z.string().nullable(),
  projectId: z.string().optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
})

export type Folder = z.infer<typeof folderSchema> & {
  children?: Folder[]
}

export const documentSchema = z.object({
  id: z.string(),
  name: z.string(),
  type: z.string(),
  size: z.number(),
  status: z.string(),
  projectId: z.string().optional(),
  folderId: z.string().nullable().optional(),
  remark: z.string().nullable().optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
})

export type ProjectDocument = z.infer<typeof documentSchema>

export const createFolderSchema = z.object({
  projectId: z.string(),
  parentId: z.string().nullable().optional(),
  name: z.string().min(1, '文件夹名称不能为空'),
})

export type CreateFolderInput = z.infer<typeof createFolderSchema>

export const moveDocumentSchema = z.object({
  folderId: z.string().nullable(),
})

export type MoveDocumentInput = z.infer<typeof moveDocumentSchema>

export const updateRemarkSchema = z.object({
  remark: z.string(),
})

export type UpdateRemarkInput = z.infer<typeof updateRemarkSchema>

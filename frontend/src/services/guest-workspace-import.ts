import {
  exportGuestWorkspace,
  recordGuestDocumentImport,
  recordGuestFolderImport,
  recordGuestLatexImport,
  recordGuestProjectImport,
  type GuestWorkspaceBundle,
} from '@/services/guest-local-workspace'
import { deleteFetch, get, post, postMultipart, put } from '@/services'
import { DOCUMENTS, FOLDERS, PROJECTS, WRITING_STATE } from '@/services/api-routes'
import { LATEX } from '@/services/api-routes'
import type { Folder, ProjectDocument } from '@/types/files'
import type { Project } from '@/types/projects'
import type { SaveWritingState } from '@/services/writing-state-api'

export type GuestImportProgress = {
  projectName: string
  completed: number
  total: number
  itemName: string
}

export type GuestImportSummary = {
  importedProjects: Array<{ localId: string; cloudId: string; name: string }>
  completed: number
  total: number
}

function countImportItems(bundle: GuestWorkspaceBundle) {
  return bundle.projects.reduce((total, entry) => total + 1 + (entry.writingState ? 1 : 0) + entry.folders.length + entry.documents.length + entry.latexFiles.length + Number(entry.project.is_deleted), 0)
}

function folderOrder(folders: Folder[]) {
  const pending = [...folders]
  const ordered: Folder[] = []
  const known = new Set<string>()
  while (pending.length) {
    const index = pending.findIndex((folder) => !folder.parentId || known.has(folder.parentId))
    if (index < 0) throw new Error('游客文件夹层级存在循环或缺失父文件夹，数据仍保留在本机。')
    const [folder] = pending.splice(index, 1)
    if (folder) {
      ordered.push(folder)
      known.add(folder.id)
    }
  }
  return ordered
}

function getProjectMapping(bundle: GuestWorkspaceBundle, accountId: string, localProjectId: string) {
  return bundle.importMappings.find((item) => item.accountId === accountId && item.localProjectId === localProjectId)
}

function importHeaders(bundleId: string, localId: string) {
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(bundleId) ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(localId)) {
    throw new Error('本地导入标识格式不受支持。请升级浏览器后重试；本机数据仍保留。')
  }
  return {
    'X-PaperSage-Import-Bundle-Id': bundleId,
    'X-PaperSage-Import-Local-Id': localId,
  }
}

function uploadedIds(bundle: GuestWorkspaceBundle, accountId: string, localProjectId: string) {
  const mapping = getProjectMapping(bundle, accountId, localProjectId)
  return {
    cloudProjectId: mapping?.cloudProjectId ?? '',
    folders: new Map((mapping?.completedFolderIds ?? []).map(({ localId, cloudId }) => [localId, cloudId])),
    documents: new Map((mapping?.completedDocumentIds ?? []).map(({ localId, cloudId }) => [localId, cloudId])),
    latexPaths: new Set(mapping?.completedLatexPaths ?? []),
  }
}

/** Import is intentionally sequential: each confirmed remote write is checkpointed before the next starts. */
export async function importGuestWorkspace(
  bundle: GuestWorkspaceBundle,
  accountId: string,
  onProgress?: (progress: GuestImportProgress) => void,
): Promise<GuestImportSummary> {
  if (bundle.bundleVersion !== 1) throw new Error('此游客数据版本暂不支持导入；本机数据没有更改。')
  const total = countImportItems(bundle)
  let completed = 0
  const importedProjects: GuestImportSummary['importedProjects'] = []
  const advance = (projectName: string, itemName: string) => {
    completed += 1
    onProgress?.({ projectName, itemName, completed, total })
  }

  for (const entry of bundle.projects) {
    const localProject = entry.project
    const existing = uploadedIds(bundle, accountId, localProject.id)
    let cloudProjectId = existing.cloudProjectId
    if (!cloudProjectId) {
      const cloudProject = await post<Project>(PROJECTS.list, {
        name: localProject.name,
        description: localProject.description ?? undefined,
        scenario: localProject.scenario,
      }, { headers: importHeaders(bundle.bundleId, localProject.id) })
      if (!cloudProject?.id) throw new Error(`项目“${localProject.name}”创建后未返回云端编号。请重试；本机数据仍保留。`)
      cloudProjectId = cloudProject.id
      await recordGuestProjectImport(bundle.bundleId, accountId, localProject.id, cloudProjectId)
    }
    importedProjects.push({ localId: localProject.id, cloudId: cloudProjectId, name: localProject.name })
    advance(localProject.name, `项目：${localProject.name}`)

    if (entry.writingState) {
      const writingState = Object.fromEntries(
        Object.entries(entry.writingState).filter(([key]) => key !== 'project_id' && key !== 'scenario'),
      ) as SaveWritingState
      await put(WRITING_STATE.project(cloudProjectId), writingState)
      advance(localProject.name, '写作计划')
    }

    const cloudFolderIds = new Map(existing.folders)
    for (const folder of folderOrder(entry.folders)) {
      if (!cloudFolderIds.has(folder.id)) {
        const cloudFolder = await post<Folder>(FOLDERS.create, {
          name: folder.name,
          project_id: cloudProjectId,
          parent_id: folder.parentId ? cloudFolderIds.get(folder.parentId) ?? null : null,
        }, { headers: importHeaders(bundle.bundleId, folder.id) })
        if (!cloudFolder?.id) throw new Error(`文件夹“${folder.name}”创建后未返回云端编号。`)
        cloudFolderIds.set(folder.id, cloudFolder.id)
        await recordGuestFolderImport(bundle.bundleId, accountId, localProject.id, folder.id, cloudFolder.id)
      }
      advance(localProject.name, `文件夹：${folder.name}`)
    }

    for (const item of entry.documents) {
      if (existing.documents.has(item.document.id)) {
        advance(localProject.name, `文档：${item.document.name}`)
        continue
      }
      const formData = new FormData()
      const file = new File([item.blob], item.document.name, { type: item.document.type || item.blob.type || 'application/octet-stream' })
      formData.append('file', file)
      formData.append('projectId', cloudProjectId)
      if (item.document.folderId) {
        const cloudFolderId = cloudFolderIds.get(item.document.folderId)
        if (!cloudFolderId) throw new Error(`文档“${item.document.name}”的本地文件夹没有导入完成。`)
        formData.append('folderId', cloudFolderId)
      }
      if (item.document.remark) formData.append('remark', item.document.remark)
      formData.append('staged', 'true')
      const staged = await postMultipart<ProjectDocument & { duplicate?: boolean }>(DOCUMENTS.upload, formData, {
        skipGlobalLoading: true,
        headers: importHeaders(bundle.bundleId, item.document.id),
      })
      if (!staged?.id) throw new Error(`文档“${item.document.name}”上传后未返回云端编号。`)
      const confirmed = await post<ProjectDocument>(DOCUMENTS.confirm(staged.id))
      if (!confirmed?.id) throw new Error(`文档“${item.document.name}”未能完成入库。`)
      await recordGuestDocumentImport(bundle.bundleId, accountId, localProject.id, item.document.id, confirmed.id)
      advance(localProject.name, `文档：${item.document.name}`)
    }

    for (const item of entry.latexFiles) {
      if (!existing.latexPaths.has(item.file.file_path)) {
        const file = item.blob
          ? new File([item.blob], item.file.file_path.split('/').pop() || item.file.file_path, { type: item.file.content_type || item.blob.type })
          : null
        const headers = importHeaders(bundle.bundleId, item.file.id)
        if (file) {
          const formData = new FormData()
          formData.append('file', file)
          formData.append('path', item.file.file_path)
          await postMultipart(LATEX.files(cloudProjectId), formData, { headers })
        } else {
          await post(LATEX.files(cloudProjectId), { path: item.file.file_path, content: item.content }, { headers })
        }
        await recordGuestLatexImport(bundle.bundleId, accountId, localProject.id, item.file.file_path)
      }
      advance(localProject.name, `LaTeX：${item.file.file_path}`)
    }

    if (localProject.is_deleted) {
      // Preserve the source trash state only after all child data is present.
      // The detail endpoint includes owned trash records, so retries can verify
      // an earlier successful delete without treating an arbitrary 404 as done.
      const cloudProject = await get<Project>(PROJECTS.detail(cloudProjectId))
      if (!cloudProject?.id) throw new Error(`无法验证项目“${localProject.name}”的云端状态，游客数据仍保留。`)
      if (!cloudProject.is_deleted) await deleteFetch(PROJECTS.detail(cloudProjectId))
      advance(localProject.name, '回收站状态')
    }
  }

  return { importedProjects, completed, total }
}

export async function getGuestImportBundle() {
  return exportGuestWorkspace()
}

export async function getAccountImportIdentity() {
  const user = await get<{ username: string; email?: string | null }>('/api/v1/auth/me', { skipGlobalLoading: true })
  if (!user?.username) throw new Error('无法读取当前账号信息，请重新登录后再试。')
  // Usernames are unique in PaperSage and available from the current /auth/me contract.
  return { accountId: user.username.toLocaleLowerCase(), username: user.username }
}

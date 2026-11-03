import type { Project, ProjectScenario } from '@/types/projects'
import type { Folder, ProjectDocument } from '@/types/files'
import type { WritingState, SaveWritingState } from '@/services/writing-state-api'
import type { LatexFileItem, LatexFileContent } from '@/services/latex-api'

const DB_NAME = 'papersage-local-guest'
const DB_VERSION = 2
const BUNDLE_VERSION = 1 as const

type StoreName = 'meta' | 'projects' | 'writingStates' | 'folders' | 'documents' | 'latexFiles' | 'importMappings'

interface StoredDocument extends ProjectDocument {
  projectId: string
  blob: Blob
}

interface StoredLatexFile extends LatexFileItem {
  projectId: string
  content: string
  blob?: Blob
}

export interface GuestImportMapping {
  bundleId: string
  accountId: string
  localProjectId: string
  cloudProjectId: string
  completedAt: string
  completedFolderIds: Array<{ localId: string; cloudId: string }>
  completedDocumentIds: Array<{ localId: string; cloudId: string }>
  completedLatexPaths: string[]
}

export interface GuestWorkspaceBundle {
  bundleVersion: typeof BUNDLE_VERSION
  bundleId: string
  exportedAt: string
  projects: Array<{
    project: Project
    writingState: WritingState | null
    folders: Folder[]
    documents: Array<{ document: ProjectDocument; blob: Blob }>
    latexFiles: Array<{ file: LatexFileItem; content: string; blob?: Blob }>
  }>
  importMappings: GuestImportMapping[]
}

export type GuestImportEntityKind = 'project' | 'folder' | 'document' | 'latex-file' | 'writing-state'

/** Stable server idempotency value for resumable account import calls. */
export function guestImportIdempotencyKey(bundleId: string, kind: GuestImportEntityKind, localId: string) {
  return `guest-import:${bundleId}:${kind}:${localId}`
}

export function guestImportHeaders(bundleId: string, localId: string): Record<string, string> {
  return {
    'X-PaperSage-Import-Bundle-Id': bundleId,
    'X-PaperSage-Import-Local-Id': localId,
  }
}

export class GuestStorageError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options)
    this.name = 'GuestStorageError'
  }
}

export class GuestCapabilityError extends Error {
  constructor(capability: 'AI 助手' | 'LaTeX 在线编译' | '云端 PDF' | '项目分组') {
    super(capability === '项目分组'
      ? '游客模式暂不支持项目分组；项目和文件仍会保存在此浏览器。'
      : `${capability}需要登录后连接云端服务。游客模式下，项目和文件仅保存在此浏览器。`)
    this.name = 'GuestCapabilityError'
  }
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onabort = () => reject(transaction.error)
    transaction.onerror = () => reject(transaction.error)
  })
}

function normalizeStorageError(error: unknown): GuestStorageError {
  if (error instanceof GuestStorageError) return error
  const name = error instanceof DOMException ? error.name : ''
  if (name === 'QuotaExceededError') {
    return new GuestStorageError('浏览器存储空间不足。请删除不需要的文件或登录后迁移项目，再继续保存。', { cause: error })
  }
  return new GuestStorageError('无法访问此浏览器的本地存储。请允许站点使用 IndexedDB，并避免无痕/隐私限制模式。', { cause: error })
}

function openDatabase(): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new GuestStorageError('此浏览器不支持本地游客工作区（IndexedDB）。请使用支持本地存储的浏览器。'))
  }
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      const ensureIndex = (store: IDBObjectStore, name: string, keyPath: string | string[]) => {
        if (!store.indexNames.contains(name)) store.createIndex(name, keyPath, { unique: name === 'projectPath' })
      }
      if (!db.objectStoreNames.contains('meta')) db.createObjectStore('meta', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('projects')) db.createObjectStore('projects', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('writingStates')) db.createObjectStore('writingStates', { keyPath: 'project_id' })
      const folders = db.objectStoreNames.contains('folders')
        ? request.transaction!.objectStore('folders')
        : db.createObjectStore('folders', { keyPath: 'id' })
      ensureIndex(folders, 'projectId', 'projectId')
      const documents = db.objectStoreNames.contains('documents')
        ? request.transaction!.objectStore('documents')
        : db.createObjectStore('documents', { keyPath: 'id' })
      ensureIndex(documents, 'projectId', 'projectId')
      ensureIndex(documents, 'folderId', 'folderId')
      const latexFiles = db.objectStoreNames.contains('latexFiles')
        ? request.transaction!.objectStore('latexFiles')
        : db.createObjectStore('latexFiles', { keyPath: 'id' })
      ensureIndex(latexFiles, 'projectPath', ['projectId', 'file_path'])
      ensureIndex(latexFiles, 'projectId', 'projectId')
      const importMappings = db.objectStoreNames.contains('importMappings')
        ? request.transaction!.objectStore('importMappings')
        : db.createObjectStore('importMappings', { keyPath: 'id' })
      ensureIndex(importMappings, 'accountBundle', ['accountId', 'bundleId'])
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(normalizeStorageError(request.error))
    request.onblocked = () => reject(new GuestStorageError('本地游客工作区正在被其他标签页升级，请关闭其他 PaperSage 标签页后重试。'))
  })
}

/** Open/create the local guest stores before activating guest navigation. */
export async function initializeGuestWorkspace(): Promise<void> {
  // Persist a tiny metadata marker so blocked/quota-full storage is detected
  // before the visitor enters the workspace; this does not create a project.
  await getOrCreateBundleId()
}

async function withStore<T>(stores: StoreName | StoreName[], mode: IDBTransactionMode, run: (tx: IDBTransaction) => Promise<T>): Promise<T> {
  let db: IDBDatabase | undefined
  try {
    db = await openDatabase()
    const tx = db.transaction(stores, mode)
    const done = transactionDone(tx)
    const result = await run(tx)
    await done
    return result
  } catch (error) {
    throw normalizeStorageError(error)
  } finally {
    db?.close()
  }
}

function newId() {
  const browserCrypto: Crypto | undefined = typeof globalThis.crypto === 'undefined' ? undefined : globalThis.crypto
  if (browserCrypto?.randomUUID) return browserCrypto.randomUUID()
  const bytes = new Uint8Array(16)
  if (browserCrypto) browserCrypto.getRandomValues(bytes)
  else for (let i = 0; i < bytes.length; i += 1) bytes[i] = Math.floor(Math.random() * 256)
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function now() { return new Date().toISOString() }

function documentMetadata(document: StoredDocument): ProjectDocument {
  const result = { ...document }
  delete (result as Partial<StoredDocument>).blob
  return result
}

function latexMetadata(file: StoredLatexFile): LatexFileItem {
  const result = { ...file }
  delete (result as Partial<StoredLatexFile>).projectId
  delete (result as Partial<StoredLatexFile>).content
  delete (result as Partial<StoredLatexFile>).blob
  return result
}

function emptyWritingState(projectId: string, scenario: ProjectScenario): WritingState {
  return {
    project_id: projectId,
    scenario,
    brief: {
      topic: '', background: '', questions: [], goals: [], scope_out: [], objects_and_data: '', methods: '',
      key_chapters: [], milestones: [], constraints: [], known_facts: [], assumptions: [], unverified: [],
    },
    requirements: [],
    phases: { prepare: 'not_started', write: 'not_started', review: 'not_started' },
    current_phase: 'prepare', revision: 1,
  }
}

const MAIN_TEMPLATE = `% !TEX program = xelatex
\\documentclass[UTF8,a4paper,12pt]{ctexart}
\\usepackage{geometry}
\\geometry{margin=2.5cm}
\\title{论文标题}
\\author{作者姓名}
\\date{\\today}
\\begin{document}
\\maketitle
\\section{引言}
从这里开始撰写。
\\end{document}
`

export async function listGuestProjects(groupId?: string, includeDeleted = false): Promise<Project[]> {
  return withStore('projects', 'readonly', async (tx) => {
    const rows = await requestResult(tx.objectStore('projects').getAll()) as Project[]
    return rows.filter((project) => (includeDeleted || !project.is_deleted) && (!groupId || project.group_id === groupId))
      .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
  })
}

export async function getGuestProject(projectId: string): Promise<Project> {
  return withStore('projects', 'readonly', async (tx) => {
    const project = await requestResult(tx.objectStore('projects').get(projectId)) as Project | undefined
    if (!project || project.is_deleted) throw new Error('找不到该项目，或项目已移至回收站。')
    return project
  })
}

export async function createGuestProject(input: { name: string; description?: string; group_id?: string; scenario?: ProjectScenario }): Promise<Project> {
  const id = newId()
  const createdAt = now()
  const project: Project = {
    id, name: input.name.trim(), description: input.description?.trim() || null, owner_id: 'guest-local',
    group_id: null, status: 'active', scenario: input.scenario ?? 'undergraduate_thesis', is_deleted: false,
    deleted_at: null, created_at: createdAt, updated_at: createdAt,
  }
  return withStore(['projects', 'writingStates', 'latexFiles'], 'readwrite', async (tx) => {
    tx.objectStore('projects').add(project)
    tx.objectStore('writingStates').add(emptyWritingState(id, project.scenario))
    tx.objectStore('latexFiles').add({
      id: newId(), projectId: id, file_path: 'main.tex', content_type: 'text/x-tex',
      size_bytes: MAIN_TEMPLATE.length, last_saved_at: createdAt, content: MAIN_TEMPLATE,
    } satisfies StoredLatexFile)
    return project
  })
}

export async function updateGuestProject(projectId: string, patch: { name?: string; description?: string; group_id?: string | null; status?: string }): Promise<Project> {
  return withStore('projects', 'readwrite', async (tx) => {
    const store = tx.objectStore('projects')
    const project = await requestResult(store.get(projectId)) as Project | undefined
    if (!project || project.is_deleted) throw new Error('找不到该项目。')
    const updated = { ...project, ...patch, updated_at: now() }
    store.put(updated)
    return updated
  })
}

export async function deleteGuestProject(projectId: string): Promise<void> {
  return withStore('projects', 'readwrite', async (tx) => {
    const store = tx.objectStore('projects')
    const project = await requestResult(store.get(projectId)) as Project | undefined
    if (!project) return
    store.put({ ...project, is_deleted: true, deleted_at: now(), updated_at: now() })
  })
}

export async function restoreGuestProject(projectId: string): Promise<void> {
  return withStore('projects', 'readwrite', async (tx) => {
    const store = tx.objectStore('projects')
    const project = await requestResult(store.get(projectId)) as Project | undefined
    if (project) store.put({ ...project, is_deleted: false, deleted_at: null, updated_at: now() })
  })
}

export async function permanentlyDeleteGuestProject(projectId: string): Promise<void> {
  return withStore(['projects', 'writingStates', 'folders', 'documents', 'latexFiles'], 'readwrite', async (tx) => {
    tx.objectStore('projects').delete(projectId)
    tx.objectStore('writingStates').delete(projectId)
    for (const storeName of ['folders', 'documents', 'latexFiles'] as const) {
      const store = tx.objectStore(storeName)
      const rows = await requestResult(store.index('projectId').getAll(projectId)) as Array<{ id: string }>
      rows.forEach((row) => store.delete(row.id))
    }
  })
}

export async function getGuestWritingState(projectId: string): Promise<WritingState> {
  return withStore(['writingStates', 'projects'], 'readonly', async (tx) => {
    const project = await requestResult(tx.objectStore('projects').get(projectId)) as Project | undefined
    if (!project || project.is_deleted) throw new Error('找不到该项目。')
    const state = await requestResult(tx.objectStore('writingStates').get(projectId)) as WritingState | undefined
    return state ?? emptyWritingState(projectId, project.scenario)
  })
}

export async function saveGuestWritingState(projectId: string, input: SaveWritingState): Promise<WritingState> {
  return withStore(['writingStates', 'projects'], 'readwrite', async (tx) => {
    const project = await requestResult(tx.objectStore('projects').get(projectId)) as Project | undefined
    if (!project || project.is_deleted) throw new Error('找不到该项目。')
    const current = await requestResult(tx.objectStore('writingStates').get(projectId)) as WritingState | undefined
    const saved: WritingState = { ...input, project_id: projectId, scenario: project.scenario, revision: (current?.revision ?? 0) + 1 }
    tx.objectStore('writingStates').put(saved)
    return saved
  })
}

export async function listGuestFolders(projectId: string): Promise<Folder[]> {
  return withStore('folders', 'readonly', async (tx) => requestResult(tx.objectStore('folders').index('projectId').getAll(projectId)) as Promise<Folder[]>)
}

export async function createGuestFolder(input: { projectId: string; parentId?: string | null; name: string }): Promise<Folder> {
  const timestamp = now()
  const folder: Folder = { id: newId(), projectId: input.projectId, parentId: input.parentId ?? null, name: input.name.trim(), createdAt: timestamp, updatedAt: timestamp }
  return withStore('folders', 'readwrite', async (tx) => { tx.objectStore('folders').add(folder); return folder })
}

export async function deleteGuestFolder(folderId: string): Promise<void> {
  return withStore(['folders', 'documents'], 'readwrite', async (tx) => {
    const folders = tx.objectStore('folders')
    const folder = await requestResult(folders.get(folderId)) as Folder | undefined
    if (!folder) return
    const all = await requestResult(folders.index('projectId').getAll(folder.projectId)) as Folder[]
    const descendantIds = new Set([folderId])
    let changed = true
    while (changed) {
      changed = false
      for (const row of all) if (row.parentId && descendantIds.has(row.parentId) && !descendantIds.has(row.id)) { descendantIds.add(row.id); changed = true }
    }
    for (const id of descendantIds) folders.delete(id)
    const docs = await requestResult(tx.objectStore('documents').index('projectId').getAll(folder.projectId)) as StoredDocument[]
    for (const document of docs) if (document.folderId && descendantIds.has(document.folderId)) tx.objectStore('documents').delete(document.id)
  })
}

// folderId 省略表示整个项目（不限目录）；显式 null 表示根目录。
export async function listGuestDocuments(projectId: string, folderId?: string | null): Promise<ProjectDocument[]> {
  return withStore('documents', 'readonly', async (tx) => {
    const docs = await requestResult(tx.objectStore('documents').index('projectId').getAll(projectId)) as StoredDocument[]
    return docs.filter((doc) => folderId === undefined || (doc.folderId ?? null) === folderId).map(documentMetadata)
  })
}

export async function saveGuestDocument(projectId: string, file: File, folderId: string | null, remark?: string): Promise<ProjectDocument> {
  const timestamp = now()
  const document: StoredDocument = {
    id: newId(), name: file.name, type: file.type || 'application/octet-stream', size: file.size,
    status: 'uploaded', projectId, folderId, remark: remark ?? null, createdAt: timestamp, updatedAt: timestamp, blob: file,
  }
  return withStore(['documents', 'projects'], 'readwrite', async (tx) => {
    const project = await requestResult(tx.objectStore('projects').get(projectId)) as Project | undefined
    if (!project || project.is_deleted) throw new Error('找不到该项目。')
    tx.objectStore('documents').add(document)
    return documentMetadata(document)
  })
}

export async function deleteGuestDocument(documentId: string): Promise<void> {
  return withStore('documents', 'readwrite', async (tx) => { tx.objectStore('documents').delete(documentId) })
}

export async function moveGuestDocument(documentId: string, folderId: string | null): Promise<ProjectDocument> {
  return withStore('documents', 'readwrite', async (tx) => {
    const store = tx.objectStore('documents')
    const doc = await requestResult(store.get(documentId)) as StoredDocument | undefined
    if (!doc) throw new Error('找不到该资料。')
    const updated = { ...doc, folderId, updatedAt: now() }
    store.put(updated)
    return documentMetadata(updated)
  })
}

export async function updateGuestDocumentRemark(documentId: string, remark: string): Promise<ProjectDocument> {
  return withStore('documents', 'readwrite', async (tx) => {
    const store = tx.objectStore('documents')
    const doc = await requestResult(store.get(documentId)) as StoredDocument | undefined
    if (!doc) throw new Error('找不到该资料。')
    const updated = { ...doc, remark, updatedAt: now() }
    store.put(updated)
    return documentMetadata(updated)
  })
}

export async function confirmGuestDocument(documentId: string): Promise<ProjectDocument> {
  return withStore('documents', 'readwrite', async (tx) => {
    const store = tx.objectStore('documents')
    const doc = await requestResult(store.get(documentId)) as StoredDocument | undefined
    if (!doc) throw new Error('找不到该资料。')
    const updated = { ...doc, status: 'uploaded', updatedAt: now() }
    store.put(updated)
    return documentMetadata(updated)
  })
}

export async function getGuestDocumentBlob(documentId: string): Promise<Blob> {
  return withStore('documents', 'readonly', async (tx) => {
    const doc = await requestResult(tx.objectStore('documents').get(documentId)) as StoredDocument | undefined
    if (!doc) throw new Error('找不到该资料。')
    return doc.blob
  })
}

export async function listGuestLatexFiles(projectId: string): Promise<LatexFileItem[]> {
  return withStore('latexFiles', 'readonly', async (tx) => {
    const files = await requestResult(tx.objectStore('latexFiles').index('projectId').getAll(projectId)) as StoredLatexFile[]
    return files.sort((a, b) => a.file_path.localeCompare(b.file_path)).map(latexMetadata)
  })
}

export async function getGuestLatexFile(projectId: string, path: string): Promise<LatexFileContent> {
  return withStore('latexFiles', 'readonly', async (tx) => {
    const file = await requestResult(tx.objectStore('latexFiles').index('projectPath').get([projectId, path])) as StoredLatexFile | undefined
    if (!file) throw new Error('找不到该 LaTeX 文件。')
    return { path, content: file.content }
  })
}

export async function saveGuestLatexFile(projectId: string, path: string, content: string): Promise<void> {
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    const existing = await requestResult(store.index('projectPath').get([projectId, path])) as StoredLatexFile | undefined
    const timestamp = now()
    store.put({ id: existing?.id ?? newId(), projectId, file_path: path, content_type: 'text/plain', size_bytes: new Blob([content]).size, last_saved_at: timestamp, content } satisfies StoredLatexFile)
  })
}

export async function createGuestLatexFile(projectId: string, path: string, content = ''): Promise<LatexFileItem> {
  const filePath = path.replace(/^\/+/, '').replace(/\\/g, '/')
  if (!filePath || filePath.split('/').some((part) => !part || part === '.' || part === '..')) throw new Error('文件路径无效。')
  const file: StoredLatexFile = { id: newId(), projectId, file_path: filePath, content_type: 'text/plain', size_bytes: new Blob([content]).size, last_saved_at: now(), content }
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    if (await requestResult(store.index('projectPath').get([projectId, filePath]))) throw new Error('该文件路径已存在。')
    store.add(file)
    return latexMetadata(file)
  })
}

export async function createGuestLatexBlobFile(projectId: string, path: string, blob: File): Promise<LatexFileItem> {
  const filePath = path.replace(/^\/+/, '').replace(/\\/g, '/')
  if (!filePath || filePath.split('/').some((part) => !part || part === '.' || part === '..')) throw new Error('文件路径无效。')
  const content = blob.type.startsWith('text/') || /\.(tex|bib|cls|sty|bst|def|txt|md|json|ya?ml|csv)$/i.test(filePath)
    ? await blob.text()
    : ''
  const file: StoredLatexFile = { id: newId(), projectId, file_path: filePath, content_type: blob.type || 'application/octet-stream', size_bytes: blob.size, last_saved_at: now(), content, blob }
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    if (await requestResult(store.index('projectPath').get([projectId, filePath]))) throw new Error('该文件路径已存在。')
    store.add(file)
    return latexMetadata(file)
  })
}

export async function getGuestLatexBlob(projectId: string, path: string): Promise<Blob | null> {
  return withStore('latexFiles', 'readonly', async (tx) => {
    const file = await requestResult(tx.objectStore('latexFiles').index('projectPath').get([projectId, path])) as StoredLatexFile | undefined
    return file?.blob ?? (file ? new Blob([file.content], { type: file.content_type }) : null)
  })
}

async function renameGuestLatexPaths(projectId: string, oldPath: string, newPath: string, folder: boolean) {
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    const rows = await requestResult(store.index('projectId').getAll(projectId)) as StoredLatexFile[]
    const changed = rows.filter((file) => folder ? file.file_path.startsWith(`${oldPath}/`) : file.file_path === oldPath)
    if (!changed.length) throw new Error('找不到要重命名的 LaTeX 路径。')
    const destinations = changed.map((file) => `${newPath}${file.file_path.slice(oldPath.length)}`)
    for (const target of destinations) {
      if (rows.some((file) => file.file_path === target && !changed.includes(file))) throw new Error('目标路径已存在。')
    }
    changed.forEach((file, index) => store.put({ ...file, file_path: destinations[index], last_saved_at: now() }))
    return changed.length
  })
}

export async function renameGuestLatexFile(projectId: string, oldPath: string, newPath: string): Promise<LatexFileItem> {
  await renameGuestLatexPaths(projectId, oldPath, newPath, false)
  return (await listGuestLatexFiles(projectId)).find((file) => file.file_path === newPath)!
}

export function renameGuestLatexFolder(projectId: string, oldPath: string, newPath: string) {
  return renameGuestLatexPaths(projectId, oldPath, newPath, true).then((renamed_count) => ({ renamed_count, old_folder: oldPath, new_folder: newPath }))
}

export async function moveGuestLatexFile(projectId: string, path: string, newDir: string): Promise<LatexFileItem> {
  const base = path.split('/').pop() ?? path
  const target = newDir ? `${newDir.replace(/\/$/, '')}/${base}` : base
  return renameGuestLatexFile(projectId, path, target)
}

export async function deleteGuestLatexFile(projectId: string, path: string): Promise<void> {
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    const file = await requestResult(store.index('projectPath').get([projectId, path])) as StoredLatexFile | undefined
    if (file) store.delete(file.id)
  })
}

export async function deleteGuestLatexFolder(projectId: string, folderPath: string) {
  return withStore('latexFiles', 'readwrite', async (tx) => {
    const store = tx.objectStore('latexFiles')
    const files = await requestResult(store.index('projectId').getAll(projectId)) as StoredLatexFile[]
    const matches = files.filter((file) => file.file_path.startsWith(`${folderPath}/`))
    for (const file of matches) store.delete(file.id)
    return { deleted_count: matches.length, folder_path: folderPath }
  })
}

async function getOrCreateBundleId(): Promise<string> {
  return withStore('meta', 'readwrite', async (tx) => {
    const store = tx.objectStore('meta')
    const saved = await requestResult(store.get('bundleId')) as { key: string; value: string } | undefined
    if (saved) return saved.value
    const bundleId = newId()
    store.put({ key: 'bundleId', value: bundleId })
    return bundleId
  })
}

export async function exportGuestWorkspace(): Promise<GuestWorkspaceBundle> {
  const [projects, bundleId, storedMappings] = await Promise.all([
    listGuestProjects(undefined, true), getOrCreateBundleId(), withStore('importMappings', 'readonly', async (tx) => requestResult(tx.objectStore('importMappings').getAll()) as Promise<GuestImportMapping[]>),
  ])
  const importMappings = storedMappings.filter((mapping) => mapping.bundleId === bundleId)
  const entries = await Promise.all(projects.map(async (project) => {
    const [writingState, folders, docs, latexFiles] = await Promise.all([
      getGuestWritingState(project.id), listGuestFolders(project.id), withStore('documents', 'readonly', async (tx) => requestResult(tx.objectStore('documents').index('projectId').getAll(project.id)) as Promise<StoredDocument[]>),
      withStore('latexFiles', 'readonly', async (tx) => requestResult(tx.objectStore('latexFiles').index('projectId').getAll(project.id)) as Promise<StoredLatexFile[]>),
    ])
    return {
      project, writingState, folders,
      documents: docs.map((document) => ({ document: documentMetadata(document), blob: document.blob })),
      latexFiles: latexFiles.map((file) => ({ file: latexMetadata(file), content: file.content, ...(file.blob ? { blob: file.blob } : {}) })),
    }
  }))
  return { bundleVersion: BUNDLE_VERSION, bundleId, exportedAt: now(), projects: entries, importMappings }
}

async function putImportMapping(bundleId: string, accountId: string, localProjectId: string, update: (current: GuestImportMapping) => GuestImportMapping) {
  const id = `${accountId}:${bundleId}:${localProjectId}`
  return withStore('importMappings', 'readwrite', async (tx) => {
    const store = tx.objectStore('importMappings')
    const current = await requestResult(store.get(id)) as (GuestImportMapping & { id: string; bundleId: string }) | undefined
    const base = current ?? { id, bundleId, accountId, localProjectId, cloudProjectId: '', completedAt: '', completedFolderIds: [], completedDocumentIds: [], completedLatexPaths: [] }
    const updated = update(base)
    store.put(updated)
    return updated
  })
}

export function getGuestImportProgress(bundleId: string, accountId: string): Promise<GuestImportMapping[]> {
  return withStore('importMappings', 'readonly', async (tx) => {
    const all = await requestResult(tx.objectStore('importMappings').getAll()) as Array<GuestImportMapping & { bundleId: string }>
    return all.filter((item) => item.bundleId === bundleId && item.accountId === accountId)
  })
}

export function recordGuestProjectImport(bundleId: string, accountId: string, localProjectId: string, cloudProjectId: string) {
  return putImportMapping(bundleId, accountId, localProjectId, (current) => ({ ...current, cloudProjectId, completedAt: now() }))
}

export function recordGuestDocumentImport(bundleId: string, accountId: string, localProjectId: string, localDocumentId: string, cloudDocumentId: string) {
  return putImportMapping(bundleId, accountId, localProjectId, (current) => ({
    ...current,
    completedDocumentIds: [...current.completedDocumentIds.filter((item) => item.localId !== localDocumentId), { localId: localDocumentId, cloudId: cloudDocumentId }],
  }))
}

export function recordGuestFolderImport(bundleId: string, accountId: string, localProjectId: string, localFolderId: string, cloudFolderId: string) {
  return putImportMapping(bundleId, accountId, localProjectId, (current) => ({
    ...current,
    completedFolderIds: [...current.completedFolderIds.filter((item) => item.localId !== localFolderId), { localId: localFolderId, cloudId: cloudFolderId }],
  }))
}

export function recordGuestLatexImport(bundleId: string, accountId: string, localProjectId: string, localPath: string) {
  return putImportMapping(bundleId, accountId, localProjectId, (current) => ({ ...current, completedLatexPaths: [...new Set([...current.completedLatexPaths, localPath])] }))
}

export async function clearGuestWorkspace(): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  await new Promise<void>((resolve, reject) => {
    const request = indexedDB.deleteDatabase(DB_NAME)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(normalizeStorageError(request.error))
    request.onblocked = () => reject(new GuestStorageError('游客工作区仍在其他标签页中打开。请关闭相关标签页后，再确认迁移完成并清理本地副本。'))
  })
}

export async function getGuestStorageUsage(): Promise<{ usage: number | null; quota: number | null }> {
  if (typeof navigator === 'undefined' || !navigator.storage?.estimate) return { usage: null, quota: null }
  try {
    const estimate = await navigator.storage.estimate()
    return { usage: estimate.usage ?? null, quota: estimate.quota ?? null }
  } catch {
    return { usage: null, quota: null }
  }
}

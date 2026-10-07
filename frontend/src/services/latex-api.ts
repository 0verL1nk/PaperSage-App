import JSZip from 'jszip'
import { getJson, postJson, putJson, deleteFetch, downloadBlob, postMultipart, type ApiRequestConfig } from './client'
import { getAccessToken } from '@/store'
import { useSessionStore } from '@/store'
import { LATEX } from './api-routes'
import { createPdfSource } from '@/lib/pdf/range-transport'
import {
  createGuestLatexBlobFile, createGuestLatexFile, deleteGuestLatexFile, deleteGuestLatexFolder, getGuestLatexFile,
  listGuestLatexFiles, moveGuestLatexFile, renameGuestLatexFile, renameGuestLatexFolder,
  saveGuestLatexFile, GuestCapabilityError,
} from './guest-local-workspace'

function requireCloudLatex(capability: 'LaTeX 在线编译' | '云端 PDF' = 'LaTeX 在线编译') {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError(capability)
}

// --- Types ---

export interface LatexFileItem {
  id: string
  file_path: string
  content_type: string
  size_bytes: number
  last_saved_at: string
}

export interface LatexFileContent {
  path: string
  content: string
}

export interface CompileResponse {
  status: string
  build_id: string | null
  output_files: Array<{ path: string; url: string; type: string }>
  error: string | null
  logs: string | null
}

export type CompileJobStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'timeout'
  | 'cancelled'

export interface CompileJobSubmitResponse {
  job_id: string
  status: CompileJobStatus
}

export interface CompileJobStatusResponse {
  job_id: string
  status: CompileJobStatus
  build_id: string | null
  error: string | null
  logs: string | null
  root_file: string
  compiler: string
  elapsed_seconds: number | null
  created_at: string
  updated_at: string
}

export interface CompileJobListItem {
  job_id: string
  status: CompileJobStatus
  root_file: string
  compiler: string
  build_id: string | null
  error: string | null
  created_at: string
  updated_at: string
}

export interface CompileJobHistoryResponse {
  items: CompileJobListItem[]
  total: number
}

/** Range transport source for react-pdf (avoids IDM interception). */
export type PdfRangeSource = { range: unknown }

// --- API Functions ---

interface LatexFileListEnvelope {
  files: LatexFileItem[]
}

export function fetchLatexFiles(projectId: string, config?: ApiRequestConfig) {
  if (useSessionStore.getState().isGuest) return listGuestLatexFiles(projectId)
  return getJson<LatexFileListEnvelope>(LATEX.files(projectId), config)
    .then((res) => res.files)
}

export function fetchLatexFileContent(projectId: string, filePath: string, config?: ApiRequestConfig) {
  if (useSessionStore.getState().isGuest) return getGuestLatexFile(projectId, filePath)
  return getJson<LatexFileContent>(LATEX.fileContent(projectId, filePath), config)
}

export function saveLatexFile(projectId: string, filePath: string, content: string, config?: ApiRequestConfig<{ content: string }>) {
  if (useSessionStore.getState().isGuest) return saveGuestLatexFile(projectId, filePath, content)
  return putJson<void, { content: string }>(
    LATEX.fileContent(projectId, filePath),
    { content },
    config,
  )
}

export function createLatexFile(projectId: string, path: string, content?: string) {
  if (useSessionStore.getState().isGuest) return createGuestLatexFile(projectId, path, content)
  return postJson<LatexFileItem, { path: string; content?: string }>(
    LATEX.files(projectId),
    { path, content },
  )
}

export function uploadLatexFile(projectId: string, path: string, file: File) {
  if (useSessionStore.getState().isGuest) return createGuestLatexBlobFile(projectId, path, file)
  const formData = new FormData()
  formData.append('file', file)
  formData.append('path', path)
  return postMultipart<LatexFileItem>(LATEX.files(projectId), formData)
}

export function deleteLatexFile(projectId: string, filePath: string) {
  if (useSessionStore.getState().isGuest) return deleteGuestLatexFile(projectId, filePath)
  return deleteFetch(LATEX.fileContent(projectId, filePath))
}

/**
 * 删除文件夹（递归删除其下所有文件）。走专用的 folder 端点并携带
 * JSON body；旧的实现误用文件端点 DELETE /latex-files/{path}，对
 * 文件夹路径必然 404。
 */
export function deleteLatexFolder(projectId: string, folderPath: string) {
  if (useSessionStore.getState().isGuest) return deleteGuestLatexFolder(projectId, folderPath)
  return deleteFetch<{ deleted_count: number; folder_path: string }, { folder_path: string }>(
    LATEX.deleteFolder(projectId),
    { data: { folder_path: folderPath } },
  )
}

export function renameLatexFile(projectId: string, oldPath: string, newPath: string) {
  if (useSessionStore.getState().isGuest) return renameGuestLatexFile(projectId, oldPath, newPath)
  return postJson<LatexFileItem>(LATEX.renameFile(projectId, oldPath), { new_path: newPath })
}

export function renameLatexFolder(projectId: string, oldFolder: string, newFolder: string) {
  if (useSessionStore.getState().isGuest) return renameGuestLatexFolder(projectId, oldFolder, newFolder)
  return postJson<{ renamed_count: number; old_folder: string; new_folder: string }>(
    LATEX.renameFolder(projectId),
    { folder_path: oldFolder, new_folder: newFolder },
  )
}

export function moveLatexFile(projectId: string, filePath: string, newDir: string) {
  if (useSessionStore.getState().isGuest) return moveGuestLatexFile(projectId, filePath, newDir)
  return postJson<LatexFileItem>(LATEX.moveFile(projectId, filePath), { new_dir: newDir })
}

export function compileLatex(
  projectId: string,
  files: Array<{ path: string; content: string }>,
  rootFile: string,
  compiler: string,
) {
  requireCloudLatex()
  return postJson<CompileJobSubmitResponse, {
    root_file: string
    compiler: string
    files: Array<{ path: string; content: string }>
  }>(LATEX.compile(projectId), {
    root_file: rootFile,
    compiler,
    files,
  })
}

export function getCompileStatus(projectId: string, jobId: string) {
  requireCloudLatex()
  return getJson<CompileJobStatusResponse>(LATEX.compileStatus(projectId, jobId), { skipGlobalLoading: true })
}

export function cancelCompile(projectId: string, jobId: string) {
  requireCloudLatex()
  return deleteFetch(`${LATEX.compile(projectId)}?job_id=${jobId}`)
}

export function getCompileHistory(projectId: string) {
  requireCloudLatex()
  return getJson<CompileJobHistoryResponse>(LATEX.compileHistory(projectId))
}

export function getOutputZip(projectId: string, buildId: string) {
  requireCloudLatex('云端 PDF')
  return downloadBlob(LATEX.outputZip(projectId, buildId))
}

/**
 * Create a PDF range transport source for react-pdf.
 * Uses Overleaf-style PDFDataRangeTransport that makes HTTP Range requests,
 * returning 206 Partial Content. IDM does not intercept range requests.
 */
export async function getPdfRangeSource(
  projectId: string,
  buildId: string,
): Promise<PdfRangeSource> {
  requireCloudLatex('云端 PDF')
  const token = getAccessToken()
  const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
  const relativeUrl = LATEX.outputPdf(projectId, buildId)
  const url = new URL(relativeUrl, window.location.origin).href
  return createPdfSource(url, headers, buildId)
}

export async function extractPdfFromZip(blob: Blob): Promise<string> {
  const zip = await JSZip.loadAsync(blob)
  const pdfEntry = Object.values(zip.files).find(
    (f) => !f.dir && f.name.toLowerCase().endsWith('.pdf'),
  )
  if (!pdfEntry) throw new Error('No PDF file found in output zip')
  const pdfBlob = await pdfEntry.async('blob')
  return URL.createObjectURL(pdfBlob)
}

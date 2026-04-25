import JSZip from 'jszip'
import { getJson, postJson, putJson, deleteFetch, downloadBlob } from './client'

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
}

// --- API Functions ---

export function fetchLatexFiles(projectId: string) {
  return getJson<LatexFileItem[]>(`/api/v1/projects/${projectId}/latex-files`)
}

export function fetchLatexFileContent(projectId: string, filePath: string) {
  return getJson<LatexFileContent>(
    `/api/v1/projects/${projectId}/latex-files/${encodeURIComponent(filePath)}`,
  )
}

export function saveLatexFile(projectId: string, filePath: string, content: string) {
  return putJson<void, { content: string }>(
    `/api/v1/projects/${projectId}/latex-files/${encodeURIComponent(filePath)}`,
    { content },
  )
}

export function createLatexFile(projectId: string, path: string, content?: string) {
  return postJson<LatexFileItem, { path: string; content?: string }>(
    `/api/v1/projects/${projectId}/latex-files`,
    { path, content },
  )
}

export function deleteLatexFile(projectId: string, filePath: string) {
  return deleteFetch(
    `/api/v1/projects/${projectId}/latex-files/${encodeURIComponent(filePath)}`,
  )
}

export function compileLatex(
  projectId: string,
  files: Array<{ path: string; content: string }>,
  rootFile: string,
  compiler: string,
) {
  return postJson<CompileResponse, {
    root_file: string
    compiler: string
    files: Array<{ path: string; content: string }>
  }>(`/api/v1/projects/${projectId}/latex/compile`, {
    root_file: rootFile,
    compiler,
    files,
  })
}

export function getOutputZip(projectId: string, buildId: string) {
  return downloadBlob(
    `/api/v1/projects/${projectId}/latex/output/${buildId}/output.zip`,
  )
}

export async function extractPdfFromZip(blob: Blob): Promise<string> {
  const zip = await JSZip.loadAsync(blob)
  const pdfEntry = Object.values(zip.files).find(
    (f) => !f.dir && f.name.toLowerCase().endsWith('.pdf'),
  )

  if (!pdfEntry) {
    throw new Error('No PDF file found in output zip')
  }

  const pdfBlob = await pdfEntry.async('blob')
  return URL.createObjectURL(pdfBlob)
}

import { beforeEach, describe, expect, it, vi } from 'vitest'

import { DOCUMENTS, FOLDERS, LATEX, PROJECTS } from '@/services/api-routes'
import { deleteFetch, get, post, postMultipart, put } from '@/services'
import {
  recordGuestDocumentImport,
  recordGuestFolderImport,
  recordGuestLatexImport,
  recordGuestProjectImport,
} from '@/services/guest-local-workspace'
import { importGuestWorkspace } from '@/services/guest-workspace-import'
import type { GuestImportMapping, GuestWorkspaceBundle } from '@/services/guest-local-workspace'

vi.mock('@/services', () => ({
  get: vi.fn(),
  deleteFetch: vi.fn(),
  post: vi.fn(),
  postMultipart: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/services/guest-local-workspace', () => ({
  exportGuestWorkspace: vi.fn(),
  recordGuestDocumentImport: vi.fn(),
  recordGuestFolderImport: vi.fn(),
  recordGuestLatexImport: vi.fn(),
  recordGuestProjectImport: vi.fn(),
}))

const bundleId = 'b142ab80-8901-48b1-a820-b80f26d9e119'
const localProjectId = 'b142ab80-8901-48b1-a820-b80f26d9e120'
const localFolderId = 'b142ab80-8901-48b1-a820-b80f26d9e121'
const localDocumentId = 'b142ab80-8901-48b1-a820-b80f26d9e122'
const localLatexId = 'b142ab80-8901-48b1-a820-b80f26d9e123'
const localLatexBlobId = 'b142ab80-8901-48b1-a820-b80f26d9e124'
const cloudProjectId = 'c142ab80-8901-48b1-a820-b80f26d9e120'
const cloudFolderId = 'c142ab80-8901-48b1-a820-b80f26d9e121'
const cloudDocumentId = 'c142ab80-8901-48b1-a820-b80f26d9e122'
const checkpoint: GuestImportMapping = {
  bundleId, accountId: 'alice', localProjectId, cloudProjectId, completedAt: '',
  completedFolderIds: [], completedDocumentIds: [], completedLatexPaths: [],
}

function makeBundle(importMappings: GuestWorkspaceBundle['importMappings'] = []): GuestWorkspaceBundle {
  return {
    bundleVersion: 1,
    bundleId,
    exportedAt: new Date().toISOString(),
    importMappings,
    projects: [{
      project: {
        id: localProjectId, name: '本地论文', description: '草稿', owner_id: 'guest-local', group_id: null,
        status: 'active', scenario: 'undergraduate_thesis', is_deleted: false, deleted_at: null,
        created_at: '2026-10-01T00:00:00.000Z', updated_at: '2026-10-01T00:00:00.000Z',
      },
      writingState: {
        project_id: localProjectId, scenario: 'undergraduate_thesis',
        brief: { topic: '主题', background: '', questions: [], goals: [], scope_out: [], objects_and_data: '', methods: '', key_chapters: [], milestones: [], constraints: [], known_facts: [], assumptions: [], unverified: [] },
        requirements: [], phases: { prepare: 'in_progress', write: 'not_started', review: 'not_started' }, current_phase: 'prepare', revision: 2,
      },
      folders: [{ id: localFolderId, name: '资料', parentId: null, projectId: localProjectId }],
      documents: [{ document: { id: localDocumentId, name: 'source.pdf', type: 'application/pdf', size: 3, status: 'ready', projectId: localProjectId, folderId: localFolderId, remark: null, createdAt: '', updatedAt: '' }, blob: new Blob(['pdf'], { type: 'application/pdf' }) }],
      latexFiles: [
        { file: { id: localLatexId, file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 7, last_saved_at: '' }, content: '\\documentclass{article}' },
        { file: { id: localLatexBlobId, file_path: 'figures/plot.pdf', content_type: 'application/pdf', size_bytes: 3, last_saved_at: '' }, content: '', blob: new Blob(['pdf'], { type: 'application/pdf' }) },
      ],
    }],
  }
}

describe('importGuestWorkspace', () => {
  beforeEach(() => {
    vi.mocked(post).mockReset()
    vi.mocked(get).mockReset()
    vi.mocked(deleteFetch).mockReset()
    vi.mocked(postMultipart).mockReset()
    vi.mocked(put).mockReset()
    vi.mocked(recordGuestProjectImport).mockReset().mockResolvedValue(checkpoint)
    vi.mocked(recordGuestFolderImport).mockReset().mockResolvedValue(checkpoint)
    vi.mocked(recordGuestDocumentImport).mockReset().mockResolvedValue(checkpoint)
    vi.mocked(recordGuestLatexImport).mockReset().mockResolvedValue(checkpoint)
    vi.mocked(put).mockResolvedValue(undefined)
    vi.mocked(post).mockImplementation(async (url) => {
      if (url === PROJECTS.list) return { id: cloudProjectId }
      if (url === FOLDERS.create) return { id: cloudFolderId }
      if (url === DOCUMENTS.confirm(cloudDocumentId)) return { id: cloudDocumentId }
      return { id: 'created' }
    })
    vi.mocked(postMultipart).mockResolvedValue({ id: cloudDocumentId })
  })

  it('uploads each resource with stable idempotency headers and checkpoints after success', async () => {
    const onProgress = vi.fn()
    const result = await importGuestWorkspace(makeBundle(), 'alice', onProgress)

    expect(result).toMatchObject({ completed: 6, total: 6, importedProjects: [{ localId: localProjectId, cloudId: cloudProjectId, name: '本地论文' }] })
    expect(post).toHaveBeenCalledWith(PROJECTS.list, expect.objectContaining({ name: '本地论文' }), {
      headers: { 'X-PaperSage-Import-Bundle-Id': bundleId, 'X-PaperSage-Import-Local-Id': localProjectId },
    })
    expect(post).toHaveBeenCalledWith(FOLDERS.create, expect.objectContaining({ project_id: cloudProjectId, name: '资料' }), {
      headers: { 'X-PaperSage-Import-Bundle-Id': bundleId, 'X-PaperSage-Import-Local-Id': localFolderId },
    })
    expect(postMultipart).toHaveBeenCalledWith(DOCUMENTS.upload, expect.any(FormData), expect.objectContaining({
      headers: { 'X-PaperSage-Import-Bundle-Id': bundleId, 'X-PaperSage-Import-Local-Id': localDocumentId },
    }))
    expect(post).toHaveBeenCalledWith(LATEX.files(cloudProjectId), expect.objectContaining({ path: 'main.tex' }), {
      headers: { 'X-PaperSage-Import-Bundle-Id': bundleId, 'X-PaperSage-Import-Local-Id': localLatexId },
    })
    expect(postMultipart).toHaveBeenCalledWith(LATEX.files(cloudProjectId), expect.any(FormData), {
      headers: { 'X-PaperSage-Import-Bundle-Id': bundleId, 'X-PaperSage-Import-Local-Id': localLatexBlobId },
    })
    expect(recordGuestProjectImport).toHaveBeenCalledWith(bundleId, 'alice', localProjectId, cloudProjectId)
    expect(recordGuestFolderImport).toHaveBeenCalledWith(bundleId, 'alice', localProjectId, localFolderId, cloudFolderId)
    expect(recordGuestDocumentImport).toHaveBeenCalledWith(bundleId, 'alice', localProjectId, localDocumentId, cloudDocumentId)
    expect(recordGuestLatexImport).toHaveBeenCalledWith(bundleId, 'alice', localProjectId, 'main.tex')
    expect(onProgress).toHaveBeenCalledTimes(6)
  })

  it('resumes from mappings without recreating completed cloud resources', async () => {
    const mapping: GuestWorkspaceBundle['importMappings'][number] = {
      bundleId, accountId: 'alice', localProjectId, cloudProjectId, completedAt: new Date().toISOString(),
      completedFolderIds: [{ localId: localFolderId, cloudId: cloudFolderId }],
      completedDocumentIds: [{ localId: localDocumentId, cloudId: cloudDocumentId }],
      completedLatexPaths: ['main.tex', 'figures/plot.pdf'],
    }
    const result = await importGuestWorkspace(makeBundle([mapping]), 'alice')

    expect(result.completed).toBe(6)
    expect(post).not.toHaveBeenCalledWith(PROJECTS.list, expect.anything(), expect.anything())
    expect(post).not.toHaveBeenCalledWith(FOLDERS.create, expect.anything(), expect.anything())
    expect(postMultipart).not.toHaveBeenCalled()
    expect(post).not.toHaveBeenCalledWith(LATEX.files(cloudProjectId), expect.anything(), expect.anything())
    expect(put).toHaveBeenCalledOnce() // Writing-state PUT is safe to replay.
  })

  it('preserves progress checkpoints and propagates a failed item for a later retry', async () => {
    vi.mocked(post).mockImplementation(async (url) => {
      if (url === PROJECTS.list) return { id: cloudProjectId }
      if (url === FOLDERS.create) return { id: cloudFolderId }
      throw new Error('folder upload network interrupted')
    })

    await expect(importGuestWorkspace(makeBundle(), 'alice')).rejects.toThrow('folder upload network interrupted')
    expect(recordGuestProjectImport).toHaveBeenCalledOnce()
    expect(recordGuestFolderImport).toHaveBeenCalledOnce()
    expect(recordGuestDocumentImport).not.toHaveBeenCalled()
    expect(recordGuestLatexImport).not.toHaveBeenCalled()
  })

  it('preserves a trashed source project and verifies cloud ownership/state before retrying deletion', async () => {
    const trashed = makeBundle()
    trashed.projects[0]!.project.is_deleted = true
    vi.mocked(get).mockResolvedValue({ id: cloudProjectId, is_deleted: true })

    const result = await importGuestWorkspace(trashed, 'alice')

    expect(result).toMatchObject({ completed: 7, total: 7 })
    expect(get).toHaveBeenCalledWith(PROJECTS.detail(cloudProjectId))
    expect(deleteFetch).not.toHaveBeenCalled()
  })

  it('moves an imported project to the account trash only after all child content is imported', async () => {
    const trashed = makeBundle()
    trashed.projects[0]!.project.is_deleted = true
    vi.mocked(get).mockResolvedValue({ id: cloudProjectId, is_deleted: false })
    vi.mocked(deleteFetch).mockResolvedValue({ id: cloudProjectId, deleted: true })

    await importGuestWorkspace(trashed, 'alice')

    const childWrites = [
      vi.mocked(put).mock.invocationCallOrder[0],
      vi.mocked(post).mock.invocationCallOrder.find((_, index) => vi.mocked(post).mock.calls[index]?.[0] === FOLDERS.create),
      vi.mocked(postMultipart).mock.invocationCallOrder[0],
    ].filter((value): value is number => value !== undefined)
    expect(childWrites.length).toBe(3)
    expect(vi.mocked(deleteFetch).mock.invocationCallOrder[0]).toBeGreaterThan(Math.max(...childWrites))
    expect(deleteFetch).toHaveBeenCalledWith(PROJECTS.detail(cloudProjectId))
  })
})

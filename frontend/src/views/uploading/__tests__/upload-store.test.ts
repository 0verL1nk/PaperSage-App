import { describe, it, expect, beforeEach } from 'vitest'
import { useUploadStore } from '@/store/upload-store'

describe('useUploadStore', () => {
  beforeEach(() => {
    // Reset store to initial state before each test
    useUploadStore.setState({
      selectedFolderId: null,
      selectedFileIds: [],
      isUploading: false,
      uploadProgress: new Map(),
    })
  })

  it('initializes with default state', () => {
    const state = useUploadStore.getState()
    expect(state.selectedFolderId).toBeNull()
    expect(state.selectedFileIds).toEqual([])
    expect(state.isUploading).toBe(false)
    expect(state.uploadProgress.size).toBe(0)
  })

  it('setSelectedFolder sets folder and clears file selection', () => {
    useUploadStore.setState({ selectedFileIds: ['a', 'b'] })
    useUploadStore.getState().setSelectedFolder('folder-1')

    const state = useUploadStore.getState()
    expect(state.selectedFolderId).toBe('folder-1')
    expect(state.selectedFileIds).toEqual([])
  })

  it('setSelectedFolder(null) resets to root', () => {
    useUploadStore.setState({ selectedFolderId: 'folder-1' })
    useUploadStore.getState().setSelectedFolder(null)
    expect(useUploadStore.getState().selectedFolderId).toBeNull()
  })

  it('toggleFileSelection adds an id', () => {
    useUploadStore.getState().toggleFileSelection('doc-1')
    expect(useUploadStore.getState().selectedFileIds).toEqual(['doc-1'])
  })

  it('toggleFileSelection removes an existing id', () => {
    useUploadStore.setState({ selectedFileIds: ['doc-1', 'doc-2'] })
    useUploadStore.getState().toggleFileSelection('doc-1')
    expect(useUploadStore.getState().selectedFileIds).toEqual(['doc-2'])
  })

  it('selectAllFiles replaces the selection', () => {
    useUploadStore.setState({ selectedFileIds: ['old'] })
    useUploadStore.getState().selectAllFiles(['a', 'b', 'c'])
    expect(useUploadStore.getState().selectedFileIds).toEqual(['a', 'b', 'c'])
  })

  it('clearSelection empties selectedFileIds', () => {
    useUploadStore.setState({ selectedFileIds: ['a', 'b'] })
    useUploadStore.getState().clearSelection()
    expect(useUploadStore.getState().selectedFileIds).toEqual([])
  })

  it('setUploading toggles the flag', () => {
    useUploadStore.getState().setUploading(true)
    expect(useUploadStore.getState().isUploading).toBe(true)
    useUploadStore.getState().setUploading(false)
    expect(useUploadStore.getState().isUploading).toBe(false)
  })

  it('setProgress updates the progress map immutably', () => {
    useUploadStore.getState().setProgress('file-1', 50)
    const state1 = useUploadStore.getState()
    expect(state1.uploadProgress.get('file-1')).toBe(50)

    useUploadStore.getState().setProgress('file-1', 100)
    const state2 = useUploadStore.getState()
    expect(state2.uploadProgress.get('file-1')).toBe(100)
    // Map reference should change (immutable update)
    expect(state2.uploadProgress).not.toBe(state1.uploadProgress)
  })

  it('setProgress supports multiple files', () => {
    useUploadStore.getState().setProgress('file-1', 30)
    useUploadStore.getState().setProgress('file-2', 60)
    const { uploadProgress } = useUploadStore.getState()
    expect(uploadProgress.get('file-1')).toBe(30)
    expect(uploadProgress.get('file-2')).toBe(60)
    expect(uploadProgress.size).toBe(2)
  })
})

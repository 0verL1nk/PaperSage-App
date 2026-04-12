import { create } from 'zustand'

interface UploadState {
  selectedFolderId: string | null
  selectedFileIds: string[]
  isUploading: boolean
  uploadProgress: Map<string, number>

  setSelectedFolder: (id: string | null) => void
  toggleFileSelection: (id: string) => void
  selectAllFiles: (ids: string[]) => void
  clearSelection: () => void
  setUploading: (uploading: boolean) => void
  setProgress: (fileId: string, progress: number) => void
}

export const useUploadStore = create<UploadState>((set) => ({
  selectedFolderId: null,
  selectedFileIds: [],
  isUploading: false,
  uploadProgress: new Map(),

  setSelectedFolder: (id) => set({ selectedFolderId: id, selectedFileIds: [] }),
  toggleFileSelection: (id) => set((state) => ({
    selectedFileIds: state.selectedFileIds.includes(id)
      ? state.selectedFileIds.filter((fid) => fid !== id)
      : [...state.selectedFileIds, id],
  })),
  selectAllFiles: (ids) => set({ selectedFileIds: ids }),
  clearSelection: () => set({ selectedFileIds: [] }),
  setUploading: (uploading) => set({ isUploading: uploading }),
  setProgress: (fileId, progress) => set((state) => {
    const newMap = new Map(state.uploadProgress)
    newMap.set(fileId, progress)
    return { uploadProgress: newMap }
  }),
}))

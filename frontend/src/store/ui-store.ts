import { create } from 'zustand'

type UiState = {
  loadingCount: number
  loadingText: string
  showLoading: (text?: string) => void
  hideLoading: () => void
  resetLoading: () => void
}

export const useUiStore = create<UiState>((set) => ({
  loadingCount: 0,
  loadingText: '',
  showLoading: (text = '加载中...') =>
    set((state) => ({
      loadingCount: state.loadingCount + 1,
      loadingText: text,
    })),
  hideLoading: () =>
    set((state) => {
      const nextCount = Math.max(0, state.loadingCount - 1)

      return {
        loadingCount: nextCount,
        loadingText: nextCount === 0 ? '' : state.loadingText,
      }
    }),
  resetLoading: () =>
    set({
      loadingCount: 0,
      loadingText: '',
    }),
}))

export function isGlobalLoadingActive() {
  return useUiStore.getState().loadingCount > 0
}

export function getGlobalLoadingText() {
  return useUiStore.getState().loadingText
}

export function showGlobalLoading(text?: string) {
  return useUiStore.getState().showLoading(text)
}

export function hideGlobalLoading() {
  return useUiStore.getState().hideLoading()
}

export function resetGlobalLoading() {
  return useUiStore.getState().resetLoading()
}

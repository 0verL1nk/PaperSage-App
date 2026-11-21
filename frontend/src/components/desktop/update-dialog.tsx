import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  getDesktopService,
  onDesktopOpenUpdate,
  onDesktopUpdateError,
  onDesktopUpdateProgress,
  openExternalSafe,
  type DesktopUpdate,
} from '@/lib/desktop'

const RELEASE_URL = 'https://github.com/0verL1nk/PaperSage-App/releases/latest'

type UpdateDialogState =
  | { phase: 'closed' | 'checking' | 'current' | 'ready' | 'installing' }
  | { phase: 'available'; update: DesktopUpdate }
  | { phase: 'downloading'; received: number; total: number }
  | { phase: 'error'; stage: 'check' | 'download' | 'install'; detail: string }

export function UpdateDialog() {
  const { t } = useTranslation()
  const [state, setState] = useState<UpdateDialogState>({ phase: 'closed' })
  const stateRef = useRef(state)
  const generation = useRef(0)
  const pendingDiscard = useRef<Promise<void> | null>(null)

  const transition = useCallback((next: UpdateDialogState) => {
    stateRef.current = next
    setState(next)
  }, [])

  const check = useCallback(async (current: number) => {
    const service = getDesktopService()
    if (!service) return
    transition({ phase: 'checking' })
    try {
      const discard = pendingDiscard.current
      if (discard) {
        await discard
        if (pendingDiscard.current === discard) pendingDiscard.current = null
      }
      if (generation.current !== current || stateRef.current.phase !== 'checking') return
      const update = await service.checkForUpdates()
      if (generation.current !== current || stateRef.current.phase !== 'checking') return
      if (update === null) {
        transition({ phase: 'current' })
      } else if (update && typeof update.version === 'string' && update.version.trim()) {
        transition({ phase: 'available', update })
      } else {
        throw new Error(i18n.t('desktopUpdate.invalidResponse'))
      }
    } catch (error) {
      if (generation.current === current && stateRef.current.phase === 'checking') {
        transition({ phase: 'error', stage: 'check', detail: error instanceof Error ? error.message : String(error ?? '') })
      }
    }
  }, [transition])

  const open = useCallback(() => {
    if (stateRef.current.phase !== 'closed' || !getDesktopService()) return
    void check(++generation.current)
  }, [check])

  useEffect(() => {
    if (!getDesktopService()) return
    window.addEventListener('papersage:open-update', open)
    const offOpen = onDesktopOpenUpdate(open)
    const offProgress = onDesktopUpdateProgress((progress) => {
      if (stateRef.current.phase === 'downloading') transition({ phase: 'downloading', ...progress })
    })
    const offError = onDesktopUpdateError((message) => {
      if (stateRef.current.phase === 'installing') transition({ phase: 'error', stage: 'install', detail: message })
    })
    return () => {
      generation.current += 1
      window.removeEventListener('papersage:open-update', open)
      offOpen()
      offProgress()
      offError()
    }
  }, [open, transition])

  const download = async () => {
    const previous = stateRef.current
    if (previous.phase !== 'available' && !(previous.phase === 'error' && previous.stage === 'download')) return
    const service = getDesktopService()
    if (!service) return
    const current = ++generation.current
    transition({ phase: 'downloading', received: 0, total: 0 })
    try {
      await service.downloadUpdate()
      if (generation.current === current && stateRef.current.phase === 'downloading') transition({ phase: 'ready' })
    } catch (error) {
      if (generation.current === current && stateRef.current.phase === 'downloading') {
        transition({ phase: 'error', stage: 'download', detail: error instanceof Error ? error.message : String(error ?? '') })
      }
    }
  }

  const install = async () => {
    const previous = stateRef.current
    if (previous.phase !== 'ready' && !(previous.phase === 'error' && previous.stage === 'install')) return
    const service = getDesktopService()
    if (!service) return
    transition({ phase: 'installing' })
    try {
      await service.installUpdate()
    } catch {
      // The bridge can disconnect as the application exits; native errors arrive as events.
    }
  }

  const discard = () => {
    const pending = getDesktopService()?.discardUpdate()
    if (!pending) return
    pendingDiscard.current = pending.catch(() => undefined)
  }

  const close = () => {
    const current = stateRef.current
    if (current.phase === 'checking' || current.phase === 'downloading' || current.phase === 'installing') return
    if (current.phase === 'ready' || (current.phase === 'error' && current.stage === 'install')) discard()
    generation.current += 1
    transition({ phase: 'closed' })
  }

  const locked = state.phase === 'checking' || state.phase === 'downloading' || state.phase === 'installing'
  const retry = () => {
    const current = stateRef.current
    if (current.phase !== 'error') return
    if (current.stage === 'check') void check(++generation.current)
    else if (current.stage === 'download') void download()
    else void install()
  }

  return (
    <Dialog open={state.phase !== 'closed'} onOpenChange={(isOpen) => { if (!isOpen) close() }}>
      <DialogContent
        showCloseButton={false}
        className="sm:max-w-md"
        onEscapeKeyDown={(event) => { if (locked) event.preventDefault() }}
        onPointerDownOutside={(event) => { if (locked) event.preventDefault() }}
      >
        <DialogHeader>
          <DialogTitle>{t('desktopUpdate.title')}</DialogTitle>
          <DialogDescription>{t('desktopUpdate.description')}</DialogDescription>
        </DialogHeader>

        {state.phase === 'checking' && <p role="status" className="text-sm">{t('desktopUpdate.checking')}</p>}
        {state.phase === 'current' && <p role="status" className="text-sm">{t('desktopUpdate.upToDate')}</p>}
        {state.phase === 'available' && (
          <div className="space-y-2 text-sm">
            <p role="status" className="font-medium">{t('desktopUpdate.available')}</p>
            <p>{t('desktopUpdate.version', { version: state.update.version })}</p>
            {typeof state.update.name === 'string' && state.update.name && <p>{state.update.name}</p>}
            {typeof state.update.notes === 'string' && state.update.notes && (
              <div>
                <p className="font-medium">{t('desktopUpdate.notes')}</p>
                <p className="max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-muted-foreground">{state.update.notes}</p>
              </div>
            )}
            <p className="text-muted-foreground">{t('desktopUpdate.adminPermission')}</p>
          </div>
        )}
        {state.phase === 'downloading' && (
          <div className="space-y-2 text-sm">
            <p role="status">{t('desktopUpdate.downloading')}</p>
            <progress
              aria-label={t('desktopUpdate.downloading')}
              className="w-full"
              max={state.total > 0 ? state.total : 1}
              value={state.total > 0 ? state.received : undefined}
            />
            <p className="text-muted-foreground">
              {state.total > 0
                ? t('desktopUpdate.progress', { received: (state.received / 1048576).toFixed(2), percent: Math.min(100, Math.round(state.received / state.total * 100)) })
                : t('desktopUpdate.progressUnknown', { received: (state.received / 1048576).toFixed(2) })}
            </p>
          </div>
        )}
        {state.phase === 'ready' && <p role="status" className="text-sm">{t('desktopUpdate.ready')}</p>}
        {state.phase === 'installing' && <p role="status" className="text-sm">{t('desktopUpdate.installing')}</p>}
        {state.phase === 'error' && (
          <div role="alert" className="space-y-1 text-sm text-destructive">
            <p>{t(`desktopUpdate.${state.stage}Error`)}</p>
            {state.detail && <p className="break-words">{state.detail}</p>}
          </div>
        )}

        {!locked && (
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={close}>{t('desktopUpdate.close')}</Button>
            {(state.phase === 'available' || state.phase === 'error') && (
              <Button variant="outline" onClick={() => void openExternalSafe(RELEASE_URL)}>{t('desktopUpdate.openDownloadPage')}</Button>
            )}
            {state.phase === 'available' && <Button onClick={() => void download()}>{t('desktopUpdate.downloadAndInstall')}</Button>}
            {state.phase === 'ready' && <Button onClick={() => void install()}>{t('desktopUpdate.restartAndInstall')}</Button>}
            {state.phase === 'error' && <Button onClick={retry}>{t('desktopUpdate.retry')}</Button>}
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

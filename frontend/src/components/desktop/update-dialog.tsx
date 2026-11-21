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
import { getDesktopService, onDesktopOpenUpdate, openExternalSafe, type DesktopUpdate } from '@/lib/desktop'

const RELEASE_URL = 'https://github.com/0verL1nk/PaperSage-App/releases/latest'

type UpdateDialogState =
  | { phase: 'closed' | 'checking' | 'current' }
  | { phase: 'available'; update: DesktopUpdate }
  | { phase: 'error'; detail: string }

export function UpdateDialog() {
  const { t } = useTranslation()
  const [state, setState] = useState<UpdateDialogState>({ phase: 'closed' })
  const stateRef = useRef(state)
  const generation = useRef(0)

  const transition = useCallback((next: UpdateDialogState) => {
    stateRef.current = next
    setState(next)
  }, [])

  const check = useCallback(async (current: number) => {
    const service = getDesktopService()
    if (!service) return
    transition({ phase: 'checking' })
    try {
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
        transition({ phase: 'error', detail: error instanceof Error ? error.message : String(error ?? '') })
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
    return () => {
      generation.current += 1
      window.removeEventListener('papersage:open-update', open)
      offOpen()
    }
  }, [open])

  const close = () => {
    if (stateRef.current.phase === 'checking') return
    generation.current += 1
    transition({ phase: 'closed' })
  }

  const checking = state.phase === 'checking'

  return (
    <Dialog open={state.phase !== 'closed'} onOpenChange={(isOpen) => { if (!isOpen) close() }}>
      <DialogContent
        showCloseButton={false}
        className="sm:max-w-md"
        onEscapeKeyDown={(event) => { if (checking) event.preventDefault() }}
        onPointerDownOutside={(event) => { if (checking) event.preventDefault() }}
      >
        <DialogHeader>
          <DialogTitle>{t('desktopUpdate.title')}</DialogTitle>
          <DialogDescription>{t('desktopUpdate.description')}</DialogDescription>
        </DialogHeader>

        {checking && <p role="status" className="text-sm">{t('desktopUpdate.checking')}</p>}
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
            <p className="text-muted-foreground">{t('desktopUpdate.manualInstall')}</p>
          </div>
        )}
        {state.phase === 'error' && (
          <div role="alert" className="space-y-1 text-sm text-destructive">
            <p>{t('desktopUpdate.checkError')}</p>
            {state.detail && <p className="break-words">{state.detail}</p>}
          </div>
        )}

        <DialogFooter className="gap-2">
          {!checking && <Button variant="outline" onClick={close}>{t('desktopUpdate.close')}</Button>}
          {state.phase === 'available' && (
            <Button onClick={() => void openExternalSafe(RELEASE_URL)}>{t('desktopUpdate.openDownloadPage')}</Button>
          )}
          {state.phase === 'error' && <Button onClick={() => void check(generation.current)}>{t('desktopUpdate.retry')}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

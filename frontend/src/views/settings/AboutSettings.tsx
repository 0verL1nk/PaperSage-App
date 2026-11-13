import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getOptionalApiBaseUrl } from '@/lib/env'
import {
  getDesktopPlatform,
  getDesktopService,
  isDesktop,
  type DesktopAppInfo,
} from '@/lib/desktop'

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className={mono ? 'text-right font-mono break-all' : 'text-right'}>{value}</span>
    </div>
  )
}

export function AboutSettings() {
  const { t } = useTranslation()
  const desktop = isDesktop()
  const [info, setInfo] = useState<DesktopAppInfo | null>(null)

  useEffect(() => {
    if (!desktop) return
    let cancelled = false
    void getDesktopService()?.appInfo().then((value) => {
      if (!cancelled) setInfo(value)
    })
    return () => {
      cancelled = true
    }
  }, [desktop])

  const buildVersion = typeof __APP_VERSION__ === 'string' ? __APP_VERSION__ : 'dev'
  const version = info?.version || buildVersion
  const platform = info?.platform || getDesktopPlatform() || navigator.platform || 'web'
  const apiBaseUrl = getOptionalApiBaseUrl() || t('settings.aboutSameOrigin')

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.aboutTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <InfoRow
            label={t('settings.aboutClient')}
            value={desktop ? t('settings.aboutClientDesktop') : t('settings.aboutClientWeb')}
          />
          <InfoRow label={t('settings.aboutVersion')} value={version} />
          <InfoRow label={t('settings.aboutPlatform')} value={platform} />
          <InfoRow label={t('settings.aboutApiBaseUrl')} value={apiBaseUrl} mono />
        </CardContent>
      </Card>
      {!desktop && (
        <p className="text-sm text-muted-foreground">{t('settings.aboutWebNote')}</p>
      )}
    </div>
  )
}

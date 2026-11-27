import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { LoadingState, PageContainer, PageHeader } from '@/components/shared'
import { useSettingsStore } from '@/store/settings-store'
import { GeneralSettings } from './GeneralSettings'
import { EditorSettings } from './EditorSettings'
import { AgentSettings } from './AgentSettings'
import { AccountSettings } from './AccountSettings'
import { AboutSettings } from './AboutSettings'

export function SettingsPage() {
  const { t } = useTranslation()
  const { settings, fetchSettings, loading } = useSettingsStore()

  useEffect(() => {
    if (!settings) {
      fetchSettings().catch(() => {})
    }
  }, [settings, fetchSettings])

  if (loading && !settings) {
    return (
      <PageContainer size="narrow">
        <LoadingState title={t('common.loadingSettings')} variant="spinner" compact />
      </PageContainer>
    )
  }

  return (
    <PageContainer size="narrow" className="space-y-6">
      <PageHeader title={t('settings.pageTitle')} />
      <Tabs defaultValue="ui" className="w-full">
        <TabsList className="mb-4">
          <TabsTrigger value="ui">{t('settings.tabGeneral')}</TabsTrigger>
          <TabsTrigger value="editor">{t('settings.tabEditor')}</TabsTrigger>
          <TabsTrigger value="agent">{t('settings.tabAgent')}</TabsTrigger>
          <TabsTrigger value="account">{t('settings.tabAccount')}</TabsTrigger>
          <TabsTrigger value="about">{t('settings.tabAbout')}</TabsTrigger>
        </TabsList>
        <TabsContent value="ui">
          <GeneralSettings />
        </TabsContent>
        <TabsContent value="editor">
          <EditorSettings />
        </TabsContent>
        <TabsContent value="agent">
          <AgentSettings />
        </TabsContent>
        <TabsContent value="account">
          <AccountSettings />
        </TabsContent>
        <TabsContent value="about">
          <AboutSettings />
        </TabsContent>
      </Tabs>
    </PageContainer>
  )
}

import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSettingsStore } from '@/store/settings-store'
import { GeneralSettings } from './GeneralSettings'
import { EditorSettings } from './EditorSettings'
import { AgentSettings } from './AgentSettings'
import { AccountSettings } from './AccountSettings'
import { AboutSettings } from './AboutSettings'
import { AiUsageSettings } from './AiUsageSettings'

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
      <div className="flex h-full items-center justify-center">
        <p className="text-muted-foreground">{t('common.loadingSettings')}</p>
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-3xl p-8 md:p-12">
      <h1 className="text-2xl font-bold mb-6">{t('settings.pageTitle')}</h1>
      <Tabs defaultValue="ui" className="w-full">
        <TabsList className="mb-4 h-auto w-full flex-wrap justify-start">
          <TabsTrigger value="ui">{t('settings.tabGeneral')}</TabsTrigger>
          <TabsTrigger value="editor">{t('settings.tabEditor')}</TabsTrigger>
          <TabsTrigger value="agent">{t('settings.tabAgent')}</TabsTrigger>
          <TabsTrigger value="aiUsage">{t('settings.tabAiUsage')}</TabsTrigger>
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
        <TabsContent value="aiUsage">
          <AiUsageSettings />
        </TabsContent>
        <TabsContent value="account">
          <AccountSettings />
        </TabsContent>
        <TabsContent value="about">
          <AboutSettings />
        </TabsContent>
      </Tabs>
    </div>
  )
}

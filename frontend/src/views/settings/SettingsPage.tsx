import { useEffect } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSettingsStore } from '@/store/settings-store'
import { GeneralSettings } from './GeneralSettings'
import { EditorSettings } from './EditorSettings'
import { AgentSettings } from './AgentSettings'

export function SettingsPage() {
  const { settings, fetchSettings, loading } = useSettingsStore()

  useEffect(() => {
    if (!settings) {
      fetchSettings().catch(() => {})
    }
  }, [settings, fetchSettings])

  if (loading && !settings) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-muted-foreground">Loading settings...</p>
      </div>
    )
  }

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">Settings</h1>
      <Tabs defaultValue="ui" className="w-full">
        <TabsList className="mb-4">
          <TabsTrigger value="ui">General</TabsTrigger>
          <TabsTrigger value="editor">Editor</TabsTrigger>
          <TabsTrigger value="agent">AI Assistant</TabsTrigger>
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
      </Tabs>
    </div>
  )
}

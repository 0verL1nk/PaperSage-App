import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { AppRouterProvider } from '@/app/router'
import { UpdateDialog } from '@/components/desktop/update-dialog'
import { TooltipProvider } from '@/components/ui/tooltip'
import { TopProgressBar } from '@/components/shared'
import { restoreSessionState } from '@/store'
import { ThemeProvider } from '@/app/theme-provider'
import { createQueryClient } from '@/app/query-client'
import '@/i18n'

const queryClient = createQueryClient()

restoreSessionState()

export function AppProviders() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <div className="h-[100dvh] overflow-hidden">
          <TooltipProvider delayDuration={120}>
            <AppRouterProvider queryClient={queryClient} />
            <UpdateDialog />
            <TopProgressBar />
          </TooltipProvider>
        </div>
        <Toaster />
      </ThemeProvider>
    </QueryClientProvider>
  )
}

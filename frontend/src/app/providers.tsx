import { QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { AppRouterProvider } from '@/app/router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { TopProgressBar } from '@/components/shared'
import { restoreSessionState } from '@/store'
import { ThemeProvider } from '@/app/theme-provider'
import '@/i18n'
import { toast } from 'sonner'

const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => {
      toast.error('请求失败', { description: error.message, duration: 4000 })
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: false,
    },
    mutations: {
      retry: false,
    },
  },
})

restoreSessionState()

export function AppProviders() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
      <TooltipProvider delayDuration={120}>
        <AppRouterProvider queryClient={queryClient} />
        <TopProgressBar />
        <Toaster />
      </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

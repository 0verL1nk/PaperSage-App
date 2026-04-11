import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { AppRouterProvider } from '@/app/router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { LoadingOverlay } from '@/shared/components'
import { restoreSessionState } from '@/shared/stores'

const queryClient = new QueryClient({
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
      <TooltipProvider delayDuration={120}>
        <AppRouterProvider queryClient={queryClient} />
        <LoadingOverlay />
        <Toaster />
      </TooltipProvider>
    </QueryClientProvider>
  )
}

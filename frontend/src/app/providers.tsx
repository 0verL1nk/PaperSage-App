import { QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { AppRouterProvider } from '@/app/router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { LoadingOverlay } from '@/components/shared'
import { restoreSessionState } from '@/store'
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
      <TooltipProvider delayDuration={120}>
        <AppRouterProvider queryClient={queryClient} />
        <LoadingOverlay />
        <Toaster />
      </TooltipProvider>
    </QueryClientProvider>
  )
}

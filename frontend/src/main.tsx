import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { initDesktop, isDesktop } from './lib/desktop'
import { restoreLastRoute } from './lib/last-route'

async function bootstrap() {
  if (isDesktop()) {
    document.documentElement.classList.add('desktop-client')
  }
  // Resolve the supervised local backend before the first API request.
  await initDesktop()
  restoreLastRoute()
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}

void bootstrap()

/** PDF caching feature flags — adapted from Overleaf. */
import { debugConsole } from './debug'

const hasTextEncoder = typeof TextEncoder !== 'undefined'
if (!hasTextEncoder) {
  debugConsole.warn('TextEncoder is not available. Disabling pdf-caching.')
}

const isOpera =
  typeof navigator !== 'undefined' &&
  Array.isArray((navigator as unknown as { userAgentData?: { brands: Array<{ brand: string }> } }).userAgentData?.brands) &&
  (navigator as unknown as { userAgentData: { brands: Array<{ brand: string }> } }).userAgentData.brands.some(
    (b) => b.brand === 'Opera'
  )
if (isOpera) {
  debugConsole.warn('Browser cache is limited in Opera. Disabling pdf-caching.')
}

// In our project we enable PDF caching by default (no A/B testing needed).
// Overleaf uses getMeta('ol-splitTestVariants') for feature flags;
// we simplify by always enabling the core caching to avoid IDM interception.

export const cachedUrlLookupEnabled = false // not applicable without CLSI content cache
export const prefetchingEnabled = false // keep simple for now
export const prefetchLargeEnabled = false
export const enablePdfCaching = hasTextEncoder && !isOpera
export const trackPdfDownloadEnabled = false
export const canUseClsiCache = false

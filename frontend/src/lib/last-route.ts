const LAST_ROUTE_KEY = 'papersage_last_route'

const REMEMBERED = /^\/(app|project)\//

export function saveLastRoute(pathname: string) {
  if (!REMEMBERED.test(pathname)) return
  try {
    window.localStorage.setItem(LAST_ROUTE_KEY, pathname)
  } catch {
    // Storage unavailable; the app simply opens on the default page.
  }
}

/** Reopen the last workspace page instead of the default landing page. */
export function restoreLastRoute() {
  if (window.location.pathname !== '/') return
  try {
    const saved = window.localStorage.getItem(LAST_ROUTE_KEY)
    if (saved && REMEMBERED.test(saved)) {
      window.history.replaceState(null, '', saved)
    }
  } catch {
    // Ignore unavailable storage.
  }
}

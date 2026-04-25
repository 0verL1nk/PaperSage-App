/** Browser detection — replacement for Overleaf's browser module. */
const nav = typeof navigator !== 'undefined' ? navigator : ({ userAgent: '', vendor: '', platform: '' } as Navigator)

const chromeMatch = /Chrome\/(\d+)/.exec(nav.userAgent)
const safari = !chromeMatch && /Apple Computer/.test(nav.vendor)

export default {
  mac: /Mac/.test(nav.platform || ''),
  safari: !!safari,
  chrome: !!chromeMatch,
  ios: /iPad|iPhone|iPod/.test(nav.userAgent),
}

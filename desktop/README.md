# PaperSage Desktop

Wails v3 desktop shell for the existing React frontend in `../frontend`.
It does not contain business logic; it hosts the SPA and exposes native
capabilities over time.

## Requirements

- Go 1.26+ (the module currently pins the toolchain via `go.mod`)
- WebView2 runtime on Windows
- pnpm, plus the `wails3` CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`

## Commands

Run from `desktop/`. Wails is pinned as a Go tool, so no global CLI install is
needed:

```bash
go tool wails3 task common:assets   # build ../frontend and copy dist into internal/assets/dist
go tool wails3 build                # build bin/papersage.exe
go tool wails3 task dev             # Vite dev server + Go file watching
go tool wails3 task package         # platform installer (Windows: NSIS, requires makensis)
```

Release builds strip symbols (`-trimpath -ldflags "-s -w"`) and ship minified
frontend code without source maps. Garble is opt-in only: its output is
currently flagged by Windows Defender, so it is not used by default. If you
still want it, install garble and pass `OBFUSCATED=true`; expect antivirus
false positives.

`internal/assets/dist` is generated and gitignored. The Go build embeds it,
so any production build must run the asset step first.

## Runtime data

Everything the client owns lives under the per-user config directory
(`%APPDATA%\PaperSage` on Windows):

- `papersage.db` — local SQLite database for the supervised backend
- `logs/backend.log` — backend stdout/stderr
- `window.json` — window size and position
- `sync.json` — local folder bindings and sync baselines

Deleting that folder resets the client (local projects included); the bundled
backend recreates it on the next launch.

## Verifying the real window

Automated UI checks can attach to the running WebView2 over CDP. Start the app
with a debug port, then point Playwright at it:

```bash
PAPERSAGE_DEVTOOLS_PORT=9222 ./bin/papersage.exe
```

```js
const browser = await chromium.connectOverCDP('http://127.0.0.1:9222')
```

The port is off unless the environment variable is set, so release builds keep
the standard WebView2 isolation.

## Icons

Brand sources live in `build/brand/`: `appicon.svg` (full), `appicon-small.svg` (≤32 px), `tray.svg`.
After editing a source, re-render it to 1024×1024 PNG with headless Edge, then run:

```bash
python build/brand/gen-icons.py
```

That rewrites the tiered `build/windows/icon.ico`, `build/appicon.png`, and `build/brand/tray-{16,32}.png`.
Refresh the macOS bundle with:

```bash
go tool wails3 generate icons -input build/appicon.png -macfilename build/darwin/icons.icns -windowsfilename build/windows/icon-wails.tmp.ico
rm build/windows/icon-wails.tmp.ico
```

`build/windows/icon.ico` is committed, so normal builds do not regenerate it.

## Layout

- `main.go` — process entry
- `internal/app` — application, window, lifecycle
- `internal/assets` — embedded frontend + SPA fallback handler
- `internal/backend` — managed local backend process
- `internal/foldersync` — local folder two-way sync engine
- `internal/deeplink` — `papersage://` parsing
- `internal/services` — native capabilities exposed to the web layer
- `internal/windowstate` — window size/position persistence
- `build/` — Wails platform build assets and Taskfiles

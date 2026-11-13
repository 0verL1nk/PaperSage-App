package app

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/0verL1nk/GraduateProject/desktop/internal/assets"
	"github.com/0verL1nk/GraduateProject/desktop/internal/backend"
	"github.com/0verL1nk/GraduateProject/desktop/internal/deeplink"
	"github.com/0verL1nk/GraduateProject/desktop/internal/services"
	"github.com/0verL1nk/GraduateProject/desktop/internal/updater"
	"github.com/0verL1nk/GraduateProject/desktop/internal/windowstate"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	Name       = "PaperSage"
	windowName = "main"
)

// Event names emitted to the frontend by native menu items.
const (
	EventNewProject    = "menu:new-project"
	EventImportFolder  = "menu:import-folder"
	EventExportProject = "menu:export-project"
)

// CheckForUpdates is invoked by Help > Check for Updates. It is a placeholder
// until internal/updater is wired in.
var CheckForUpdates = func() {}

// Version is injected at build time via -ldflags.
var Version = "0.0.0"

func Run() error {
	var app *application.App
	var win *application.WebviewWindow
	var supervisor *backend.Supervisor
	var desktopService *services.DesktopService
	var quitting atomic.Bool

	statePath := stateFilePath()

	showWindow := func() {
		if win == nil {
			return
		}
		win.Restore()
		win.Show()
		win.Focus()
	}
	hideWindow := func() {
		if win == nil {
			return
		}
		saveWindowState(win, statePath)
		win.Hide()
	}
	quit := func() {
		quitting.Store(true)
		if win != nil {
			saveWindowState(win, statePath)
		}
		if supervisor != nil {
			_ = supervisor.Stop()
		}
		app.Quit()
	}

	app = application.New(application.Options{
		Name:        Name,
		Description: "PaperSage desktop client",
		Assets: application.AssetOptions{
			Handler: assets.Handler(),
		},
		Windows: application.WindowsOptions{
			AdditionalBrowserArgs: devtoolsArgs(os.Getenv("PAPERSAGE_DEVTOOLS_PORT")),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.papersage.desktop",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				showWindow()
				if desktopService == nil {
					return
				}
				if path := deepLinkPath(data.Args); path != "" {
					desktopService.SetPendingDeepLink(path)
					app.Event.Emit("desktop:deeplink", path)
				}
			},
		},
	})

	// The application menu must be set before the window is created: on
	// Windows/Linux the window binds to it during construction. Windows uses a
	// custom titlebar/menu, so the native menu is only installed on macOS.
	if runtime.GOOS != "windows" {
		app.Menu.Set(newMenu(app, func() *application.WebviewWindow { return win }, showWindow, hideWindow, quit))
	}

	services.Version = Version
	desktopService = services.New(app)
	app.RegisterService(application.NewService(desktopService))
	syncService, err := services.NewSyncService(func(name string, data any) {
		app.Event.Emit(name, data)
	}, filepath.Join(filepath.Dir(statePath), "sync.json"))
	if err != nil {
		log.Printf("desktop: folder sync unavailable: %v", err)
	} else {
		app.RegisterService(application.NewService(syncService))
		syncService.ResumeWatches()
	}
	// The first launch may itself be a protocol invocation; hand the route to
	// the web layer once it mounts.
	if path := deepLinkPath(os.Args[1:]); path != "" {
		desktopService.SetPendingDeepLink(path)
	}

	if err := updater.Setup(app, Version); err != nil {
		log.Printf("desktop: updater setup: %v", err)
	}
	CheckForUpdates = func() { _ = updater.CheckNow(context.Background()) }

	supervisor = backend.New()
	if baseURL, err := supervisor.Start(context.Background()); err != nil {
		log.Printf("desktop: local backend unavailable: %v", err)
	} else {
		desktopService.SetLocalBackendURL(baseURL)
		assets.SetBackendURL(baseURL)
	}
	defer func() {
		if supervisor != nil {
			_ = supervisor.Stop()
		}
	}()

	opts := application.WebviewWindowOptions{
		Name:               windowName,
		Title:              Name,
		Width:              1280,
		Height:             800,
		MinWidth:           960,
		MinHeight:          640,
		URL:                "/",
		EnableFileDrop:     true,
		UseApplicationMenu: runtime.GOOS != "windows",
		// Windows uses a frameless window with a React titlebar. Keep WebView2
		// non-client hit testing off so the titlebar buttons receive clicks;
		// dragging is handled by --wails-draggable, double-click in React.
		Frameless: runtime.GOOS == "windows",
		Windows: application.WindowsWindow{
			NonClientRegionSupport: false,
		},
	}
	state, hasState := windowstate.Load(statePath)
	if hasState {
		opts.X, opts.Y = state.X, state.Y
		opts.Width, opts.Height = state.Width, state.Height
		opts.InitialPosition = application.WindowXY
	}

	win = app.Window.NewWithOptions(opts)
	if hasState && state.Maximised {
		win.Maximise()
	}

	// Hook (not listener) so Cancel runs before Wails' built-in close listener.
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		saveWindowState(win, statePath)
		if quitting.Load() {
			return
		}
		win.Hide()
		event.Cancel()
	})

	// Forward native drops with the data-drop-target marker so the web layer
	// can route them to the editor or the reference library.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		target := ""
		if details := event.Context().DropTargetDetails(); details != nil {
			target = details.Attributes["data-drop-target"]
			if target == "" {
				target = details.Attributes["data-file-drop-target"]
			}
		}
		app.Event.Emit("desktop:files-dropped", map[string]any{
			"files":  event.Context().DroppedFiles(),
			"target": target,
		})
	})

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon())
	tray.SetTooltip(Name)
	tray.OnClick(showWindow)

	trayMenu := app.NewMenu()
	trayMenu.Add("Show Window").OnClick(func(*application.Context) { showWindow() })
	trayMenu.Add("Hide Window").OnClick(func(*application.Context) { hideWindow() })
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) { quit() })
	tray.SetMenu(trayMenu)

	return app.Run()
}

func newMenu(
	app *application.App,
	window func() *application.WebviewWindow,
	show, hide, quit func(),
) *application.Menu {
	menu := app.NewMenu()

	file := menu.AddSubmenu("File")
	file.Add("New Project").SetAccelerator("CmdOrCtrl+n").OnClick(func(*application.Context) {
		app.Event.Emit(EventNewProject)
	})
	file.Add("Import Folder").SetAccelerator("CmdOrCtrl+o").OnClick(func(*application.Context) {
		app.Event.Emit(EventImportFolder)
	})
	file.Add("Export Project").SetAccelerator("CmdOrCtrl+e").OnClick(func(*application.Context) {
		app.Event.Emit(EventExportProject)
	})
	file.AddSeparator()
	file.Add("Quit").SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) { quit() })

	edit := menu.AddSubmenu("Edit")
	edit.AddRole(application.Undo)
	edit.AddRole(application.Redo)
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	edit.AddSeparator()
	edit.AddRole(application.SelectAll)

	view := menu.AddSubmenu("View")
	view.Add("Reload").SetAccelerator("CmdOrCtrl+r").OnClick(func(*application.Context) {
		if w := window(); w != nil {
			w.Reload()
		}
	})
	view.Add("Show/Hide Window").OnClick(func(*application.Context) {
		if w := window(); w != nil {
			if w.IsVisible() {
				hide()
			} else {
				show()
			}
		}
	})
	view.Add("Toggle Fullscreen").SetAccelerator("F11").OnClick(func(*application.Context) {
		if w := window(); w != nil {
			w.ToggleFullscreen()
		}
	})

	help := menu.AddSubmenu("Help")
	help.Add("Check for Updates").OnClick(func(*application.Context) { CheckForUpdates() })
	help.AddRole(application.About)

	return menu
}

func saveWindowState(win *application.WebviewWindow, path string) {
	x, y := win.Position()
	w, h := win.Size()
	_ = windowstate.Save(path, windowstate.State{
		X: x, Y: y, Width: w, Height: h, Maximised: win.IsMaximised(),
	})
}

// deepLinkPath maps a papersage:// invocation to the SPA route it opens.
func deepLinkPath(args []string) string {
	for _, arg := range args {
		target, err := deeplink.Parse(strings.TrimSpace(arg))
		if err != nil {
			continue
		}
		stage := target.Stage
		if stage == "" {
			stage = "prepare"
		}
		return "/project/" + target.ProjectID + "/" + stage
	}
	return ""
}

func stateFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, Name, "window.json")
}

// devtoolsArgs exposes a CDP port for automated UI verification. Wails clears
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS, so the port must be passed explicitly.
func devtoolsArgs(port string) []string {
	port = strings.TrimSpace(port)
	if port == "" {
		return nil
	}
	return []string{"--remote-debugging-port=" + port}
}

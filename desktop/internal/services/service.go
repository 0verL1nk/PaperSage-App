// Package services exposes native desktop capabilities to the web layer
// through Wails service bindings.
package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0verL1nk/GraduateProject/desktop/internal/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Version is injected at build time via -ldflags.
var Version = "0.0.0"

// DesktopService is the native bridge bound to the frontend.
type DesktopService struct {
	app             *application.App
	notifier        *notifications.NotificationService
	seq             atomic.Uint64
	localBackendURL atomic.Value // string
	pendingDeepLink atomic.Value // string
	backendError    atomic.Value // string
}

// New creates the service. Register it with application.NewService before App.Run.
func New(app *application.App) *DesktopService {
	return &DesktopService{app: app, notifier: notifications.New()}
}

// ServiceName is the stable binding name used by the frontend.
func (s *DesktopService) ServiceName() string { return "DesktopService" }

// ServiceStartup initialises the notification backend. Wails calls it on Run
// for registered services, which is when the notifier can register itself.
func (s *DesktopService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.notifier.ServiceStartup(ctx, options)
}

func (s *DesktopService) ServiceShutdown() error {
	return s.notifier.ServiceShutdown()
}

// SetLocalBackendURL records the supervised local backend address. The app
// wires this after starting the bundled server.
func (s *DesktopService) SetLocalBackendURL(rawURL string) {
	s.localBackendURL.Store(rawURL)
}

// LocalBackendURL returns the supervised backend base URL, or "".
func (s *DesktopService) LocalBackendURL() string {
	v, _ := s.localBackendURL.Load().(string)
	return v
}

// SetLocalBackendError records why the bundled backend failed to start. The
// web layer shows it once on boot so a rejected backend is never silent.
func (s *DesktopService) SetLocalBackendError(message string) {
	s.backendError.Store(message)
}

// LocalBackendError reports the bundled backend's startup failure, or "".
func (s *DesktopService) LocalBackendError() string {
	v, _ := s.backendError.Load().(string)
	return v
}

// SetPendingDeepLink records a protocol route for the web layer to consume
// once it has mounted.
func (s *DesktopService) SetPendingDeepLink(path string) {
	s.pendingDeepLink.Store(path)
}

// ConsumeDeepLink returns and clears the pending protocol route.
func (s *DesktopService) ConsumeDeepLink() string {
	v, _ := s.pendingDeepLink.Load().(string)
	s.pendingDeepLink.Store("")
	return v
}

// WindowMinimise minimises the main window.
func (s *DesktopService) WindowMinimise() {
	if w := s.app.Window.Current(); w != nil {
		w.Minimise()
	}
}

// WindowToggleMaximise toggles the maximised state.
func (s *DesktopService) WindowToggleMaximise() {
	if w := s.app.Window.Current(); w != nil {
		w.ToggleMaximise()
	}
}

// WindowClose closes (or, with close-to-tray, hides) the main window.
func (s *DesktopService) WindowClose() {
	if w := s.app.Window.Current(); w != nil {
		w.Close()
	}
}

// WindowIsMaximised reports the current maximised state.
func (s *DesktopService) WindowIsMaximised() bool {
	if w := s.app.Window.Current(); w != nil {
		return w.IsMaximised()
	}
	return false
}

// WindowToggleFullscreen toggles fullscreen mode.
func (s *DesktopService) WindowToggleFullscreen() {
	if w := s.app.Window.Current(); w != nil {
		w.ToggleFullscreen()
	}
}

// EventUpdateProgress reports installer download progress to the web layer.
const EventUpdateProgress = "desktop:update-progress"

// CheckForUpdates looks for a newer desktop release without downloading it.
// The web layer then downloads and installs it; a nil result means the current
// version is up to date.
func (s *DesktopService) CheckForUpdates() (map[string]any, error) {
	info, err := updater.Check(context.Background())
	if err != nil {
		log.Printf("desktop: update check: %v", err)
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	out := map[string]any{"version": info.Version}
	if info.Name != "" {
		out["name"] = info.Name
	}
	if info.Notes != "" {
		out["notes"] = info.Notes
	}
	return out, nil
}

// DownloadUpdate fetches and verifies the installer for the release found by
// CheckForUpdates, reporting progress as desktop:update-progress.
func (s *DesktopService) DownloadUpdate() error {
	return updater.Download(context.Background(), func(received, total int64) {
		s.app.Event.Emit(EventUpdateProgress, map[string]any{"received": received, "total": total})
	})
}

// InstallUpdate starts the detached installer helper and quits so the helper
// can replace the installed files. The helper restarts the application.
func (s *DesktopService) InstallUpdate() error {
	if err := updater.Handoff(); err != nil {
		log.Printf("desktop: install update: %v", err)
		s.app.Event.Emit(updater.EventUpdateError, map[string]string{"message": err.Error()})
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.app.Quit()
	}()
	return nil
}

// DiscardUpdate removes a downloaded installer that will not be used.
func (s *DesktopService) DiscardUpdate() error {
	return updater.Discard()
}

// QuitApp terminates the application (bypassing close-to-tray).
func (s *DesktopService) QuitApp() {
	s.app.Quit()
}

// AppInfo returns build and platform metadata for the frontend.
func (s *DesktopService) AppInfo() (map[string]any, error) {
	return map[string]any{
		"version":         Version,
		"platform":        runtime.GOOS,
		"isDesktop":       true,
		"localApiBaseUrl": s.LocalBackendURL(),
	}, nil
}

// Notify shows a desktop notification.
func (s *DesktopService) Notify(title, body string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("notification title must not be empty")
	}
	id := fmt.Sprintf("papersage-%d-%d", time.Now().UnixNano(), s.seq.Add(1))
	return s.notifier.SendNotification(notifications.NotificationOptions{
		ID:    id,
		Title: title,
		Body:  body,
	})
}

// OpenExternal opens an http, https or mailto URL in the system browser.
func (s *DesktopService) OpenExternal(rawURL string) error {
	if err := validateExternalURL(rawURL); err != nil {
		return err
	}
	return s.app.Browser.OpenURL(rawURL)
}

// PickFolder shows a directory picker. It returns "" when the user cancels.
func (s *DesktopService) PickFolder(title string) (string, error) {
	return s.app.Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}

// PickFiles shows a file picker. It returns nil when the user cancels.
func (s *DesktopService) PickFiles(title string, allowMultiple bool) ([]string, error) {
	dialog := s.app.Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		SetTitle(title)
	if allowMultiple {
		return dialog.PromptForMultipleSelection()
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	return []string{path}, nil
}

// SaveFile shows a save dialog with a suggested filename. It returns "" when
// the user cancels.
func (s *DesktopService) SaveFile(defaultName string) (string, error) {
	return s.app.Dialog.SaveFile().SetFilename(defaultName).PromptForSingleSelection()
}

// RevealInFolder opens the OS file manager with the path selected (or its
// parent opened on Linux).
func (s *DesktopService) RevealInFolder(path string) error {
	abs, err := validateAbsolutePath(path)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer.exe commonly exits non-zero even on success, so don't wait.
		cmd = exec.Command("explorer.exe", "/select,"+abs)
	case "darwin":
		cmd = exec.Command("open", "-R", abs)
	case "linux":
		cmd = exec.Command("xdg-open", filepath.Dir(abs))
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open file manager: %w", err)
	}
	return cmd.Process.Release()
}

func validateExternalURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return errors.New("URL must include a host")
		}
	case "mailto":
		if u.Opaque == "" && u.Path == "" {
			return errors.New("mailto URL must include an address")
		}
	default:
		return fmt.Errorf("unsupported URL scheme %q: only http, https and mailto are allowed", u.Scheme)
	}
	return nil
}

func validateAbsolutePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path must not be empty")
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("path must be absolute: %s", trimmed)
	}
	abs := filepath.Clean(trimmed)
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("path does not exist: %w", err)
	}
	return abs, nil
}

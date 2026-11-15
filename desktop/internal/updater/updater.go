// Package updater wires the Wails updater to the project's GitHub releases.
package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Desktop releases ship from the public app repository.
const repository = "0verL1nk/PaperSage-App"

// EventUpdateError carries updater failures to the web layer.
const EventUpdateError = "desktop:update-error"

var app *application.App

// Setup configures the application's updater. Call it once after
// application.New, before app.Run. It does not start any background check.
func Setup(a *application.App, version string) error {
	if a == nil {
		return errors.New("updater: application is nil")
	}
	provider, err := github.New(github.Config{
		Repository:    repository,
		ChecksumAsset: "SHA256SUMS",
		// Prerelease false keeps the provider on /releases/latest, which
		// excludes prereleases and drafts.
	})
	if err != nil {
		return err
	}
	err = a.Updater.Init(wailsupdater.Config{
		CurrentVersion: strings.TrimPrefix(strings.TrimSpace(version), "v"),
		Providers:      []wailsupdater.Provider{provider},
	})
	if err != nil {
		return err
	}
	// Surface every failed stage to the web layer; the frontend shows a toast
	// and falls back to a system notification when its window is inactive.
	a.Event.On(wailsupdater.EventError, func(event *application.CustomEvent) {
		if message := errorMessage(event.Data); message != "" {
			a.Event.Emit(EventUpdateError, map[string]string{"message": message})
		}
	})
	app = a
	return nil
}

// errorMessage extracts the message from an updater error payload.
func errorMessage(data any) string {
	switch v := data.(type) {
	case wailsupdater.ErrorInfo:
		return v.Message
	case *wailsupdater.ErrorInfo:
		if v != nil {
			return v.Message
		}
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
	return ""
}

// CheckNow runs a foreground check-and-install with the built-in update
// window. The caller decides when (and on which goroutine) this runs.
func CheckNow(ctx context.Context) error {
	if app == nil {
		return errors.New("updater: Setup has not been called")
	}
	return app.Updater.CheckAndInstall(ctx)
}

// State reports the updater's current lifecycle phase.
func State() wailsupdater.State {
	if app == nil {
		return wailsupdater.StateUnconfigured
	}
	return app.Updater.State()
}

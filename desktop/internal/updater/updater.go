// Package updater wires the Wails updater to the project's GitHub releases.
package updater

import (
	"context"
	"errors"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const repository = "0verL1nk/GraduateProject"

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
	app = a
	return nil
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

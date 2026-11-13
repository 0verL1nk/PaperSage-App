package app

import _ "embed"

// Wails takes raw PNG bytes; the largest icon is used so the OS can scale down
// cleanly. There is no DPI-selection API in v3.1.
//
//go:embed icons/tray-32.png
var trayIcon32 []byte

//go:embed icons/tray-16.png
var trayIcon16 []byte

func trayIcon() []byte {
	if len(trayIcon32) > 0 {
		return trayIcon32
	}
	return trayIcon16
}

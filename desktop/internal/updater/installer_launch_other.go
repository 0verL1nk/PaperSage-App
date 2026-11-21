//go:build !windows

package updater

import "errors"

func launchVerifiedInstaller(_ string, _ []byte) error {
	return errors.New("updater: installer launch is supported only on Windows")
}

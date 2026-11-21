//go:build !windows

package updater

import "errors"

func spawnUpdateHelper(_ string, _ []byte) error {
	return errors.New("updater: installer launch is supported only on Windows")
}

// RunUpdateHelper reports whether this process should run as the update helper.
func RunUpdateHelper(_ []string) (bool, error) {
	return false, nil
}

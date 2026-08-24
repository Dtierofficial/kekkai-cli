//go:build !windows

package vault

import "os"

func atomicReplace(source, destination string) error {
	return os.Rename(source, destination)
}

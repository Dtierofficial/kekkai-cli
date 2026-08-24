//go:build !windows

package main

// SetupConsole is a no-op outside Windows.
func SetupConsole() (func(), error) {
	return func() {}, nil
}

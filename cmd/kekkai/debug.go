package main

import (
	"io"
	"os"
	"sync"
)

// debugLog returns a write-through log handle only when KEKKAI_DEBUG points
// to a file path; it returns nil otherwise. Kekkai never logs keystrokes by
// default - it is a password manager, and input traces must stay opt-in.
var (
	debugOnce sync.Once
	debugFile io.Writer
)

func debugLog() io.Writer {
	debugOnce.Do(func() {
		path := os.Getenv("KEKKAI_DEBUG")
		if path == "" {
			return
		}
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
			debugFile = f
		}
	})
	return debugFile
}

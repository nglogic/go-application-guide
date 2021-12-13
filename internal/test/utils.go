package test

import (
	"path"
	"runtime"
	"strings"
)

// ProjectAbsoultePath returns absolute path to project files.
// Useful for opening test files.
func ProjectAbsoultePath() string {
	_, currFilename, _, _ := runtime.Caller(0) //nolint:dogsled
	currFileRelativePath := path.Join("internal", "test", "utils.go")
	return strings.TrimSuffix(currFilename, currFileRelativePath)
}

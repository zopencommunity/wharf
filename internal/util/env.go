// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.

// This package is deidcated to modifying and checking the environment settings
package util

import (
	"embed"
	"os"
	"strings"
)

const INTERNAL_PATH_PREFIX = "//:INTERNAL/"

// go:embed include
var included embed.FS

func ReadFile(path string) ([]byte, error) {
	if strings.HasPrefix(path, INTERNAL_PATH_PREFIX) {
		return included.ReadFile(strings.TrimPrefix(path, INTERNAL_PATH_PREFIX))
	}
	return os.ReadFile(path)
}

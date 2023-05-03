// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package porting

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/zosopentools/wharf/internal/direct"
)

type Config struct {
	// Environment according to 'go env'
	//
	// Must not be nil
	GoEnv map[string]string

	// Build tags used for determining what files to use
	//
	BuildTags []string

	// Cache directory
	Cache string

	// Directory for importing in modules
	//
	ImportDir string

	// Dry Run
	//
	DryRun bool

	// Use VCS when importing modules
	//
	UseVCS bool

	// Verbose patch instructions
	//
	Verbose bool

	// Explicitly defined handlers for packages
	//
	Directives map[string]*direct.PackageDirective
}

// Add build tags that are parsed from 'go env' fields
func parseGoEnvTags(goenv map[string]string, tags map[string]bool) error {
	// Set tag for GOARCH
	tags[goenv["GOARCH"]] = true

	// Set tag for cgo
	if goenv["CGO_ENABLED"] == "1" {
		tags["cgo"] = true
	}

	// Set compiler tag
	tags[build.Default.Compiler] = true

	// Parse go version tags
	match := regexp.MustCompile(`go1\.(\d+)(?:(?:\.|-).+)?$`).FindStringSubmatch(goenv["GOVERSION"])
	if match == nil {
		// TODO: this also breaks on dev versions
		return fmt.Errorf("unknown go version number: %v", goenv["GOVERSION"])
	}
	vnum, err := strconv.Atoi(match[1])
	if err != nil {
		panic("go version minor number unable to parse to int")
	}

	for vnum > 0 {
		tags[fmt.Sprintf("go1.%v", vnum)] = true
		vnum -= 1
	}

	return nil
}

// Setup the cache directory
//
// If the cache directory already exists reuse it
func setupCache(dir string) error {
	errorf := func(err error) error {
		return fmt.Errorf(
			"unable to setup cache directory (%v):%w",
			dir,
			err,
		)
	}

	cachedir, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.Mkdir(dir, 0755)
			if err != nil {
				return errorf(err)
			}
		} else {
			return errorf(err)
		}
	} else if !cachedir.IsDir() {
		return errorf(errors.New("a file already exists with that name"))
	}

	return nil
}

func setupTempGoWork(src string) (string, error) {
	// Copy GOWORK into a temp file called .wharf.work
	goworkfile, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf(
			"unable to open workspace (%v):\n%w",
			src,
			err,
		)
	}

	new := filepath.Join(filepath.Dir(src), ".wharf.work")
	err = os.WriteFile(new, goworkfile, 0655)
	if err != nil {
		return "", fmt.Errorf(
			"unable to create temp workspace (%v):\n%w",
			new,
			err,
		)
	}

	err = os.Setenv("GOWORK", new)
	if err != nil {
		return "", fmt.Errorf(
			"unable to update GOWORK:\n%w",
			err,
		)
	}

	return new, nil
}

// Clear the cache directory
func clearCache(dir string) error {
	err := os.RemoveAll(dir)
	if err != nil {
		return fmt.Errorf(
			"unable to delete cache directory (%v):\n%w",
			dir,
			err,
		)
	}

	return nil
}

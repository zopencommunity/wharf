// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package base

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/zosopentools/wharf/internal/util"
)

func init() {
	initInlines()
	initGoEnv()
}

func initGoEnv() {
	var err error
	goenv, err = util.GoEnv()
	if err != nil {
		panic(fmt.Sprintf("unable to inspect Go environment (cannot execute 'go env'): %v", err))
	}

	// Set tags that Go figures out from the environment, such as GOARCH, CGO, and GOVERSION
	BuildTags[goenv["GOARCH"]] = true
	BuildTags[build.Default.Compiler] = true
	if goenv["CGO_ENABLED"] == "1" {
		BuildTags["cgo"] = true
	}

	var vnum int
	if match := regexp.MustCompile(`go1\.(\d+)(?:(?:\.|-).+)?$`).FindStringSubmatch(goenv["GOVERSION"]); match != nil {
		var err error
		vnum, err = strconv.Atoi(match[1])
		if err != nil {
			panic("go version minor number unable to parse to int")
		}
	} else {
		vnum = 18
		fmt.Fprintf(os.Stderr, "unknown go version number (%v) - assuming go1.18\n", goenv["GOVERSION"])
	}

	for vnum >= 0 {
		BuildTags[fmt.Sprintf("go1.%v", vnum)] = true
		vnum -= 1
	}

	// Initialize some variables here to default values (can be overwritten)
	goWorkDir := filepath.Dir(GOWORK())
	Cache = filepath.Join(goWorkDir, ".wharf_cache") // TODO: move this to TMPDIR

	// TODO: make this relative to the position of the GOWORK folder
	// so that `go work use` uses a relative position instead of absolute
	ImportDir = filepath.Join(goWorkDir, "wharf_port")
}

var goenv = make(map[string]string)
var BuildTags = make(map[string]bool)

var ImportDir string
var Cache string

var Verbose bool
var DryRun bool
var CloneFromVCS bool
var GeneratePatches bool
var ShowDiscovery bool

func GOOS() string {
	return goenv["GOOS"]
}

func GOARCH() string {
	return goenv["GOARCH"]
}

func GOWORK() string {
	return goenv["GOWORK"]
}

func GoEnv(key string) string {
	return goenv[key]
}

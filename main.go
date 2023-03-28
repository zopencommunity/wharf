// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.ibm.com/open-z/wharf/internal/direct"
	"github.ibm.com/open-z/wharf/internal/porting"
	"github.ibm.com/open-z/wharf/internal/util"
)

var dryRunFlag *bool
var verboseFlag *bool
var testFlag *bool
var configFlag *string

var tagsFlag *string
var helpFlag *bool

func main() {
	// Parse cmd line flags
	helpFlag = flag.Bool("help", false, "Print help text")
	tagsFlag = flag.String("tags", "", "List of build tags")
	dryRunFlag = flag.Bool("n", false, "Enable dry mode, make suggestions but don't preform changes")
	verboseFlag = flag.Bool("v", false, "Enable verbose output")
	testFlag = flag.Bool("t", false, "Test the package after the porting stage")
	configFlag = flag.String("config", "", "Config for additional code edits")
	flag.Parse()

	// Turn off log flags
	log.SetFlags(0)

	// If --help is passed
	if *helpFlag {
		fmt.Println(helpText)
		os.Exit(0)
	}

	// Verify arg length
	if flag.NArg() != 1 {
		log.Fatal("No package paths provided; see 'wharf --help' for usage")
	}

	// Handle config file argument
	if *configFlag != "" {
		rawcfg, err := util.ReadFile(*configFlag)
		if err != nil {
			log.Fatal("Unable to read config file", err)
		}

		cfg, err := direct.ParseConfig(rawcfg)
		if err != nil {
			log.Fatal("Unable to parse config file", err)
		}

		direct.Apply(cfg)
	}

	paths := flag.Args()

	if err := main1(paths, *tagsFlag, *verboseFlag, *dryRunFlag); err != nil {
		fmt.Println(err.Error())
		fmt.Println("Porting failed due to errors mentioned above")
	} else {
		fmt.Println("All packages ported successfully!")
		if *testFlag {
			// Run tests
			// TODO: this could be better... such as run tests on packages we specifically touched as well
			fmt.Println("\nRunning tests...")
			if output, err := util.GoTest(paths); err != nil {
				fmt.Println("Tests failed:\n" + output)
			} else {
				fmt.Println("Tests passed!")
			}
		}
	}
}

func main1(paths []string, tags string, verbose bool, dryRun bool) error {
	// Verify that we are running in a workspace
	goenv, err := util.GoEnv()
	if err != nil {
		log.Fatal("Unable to read 'go env':", err)
	}

	var importDir string
	if gowork := goenv["GOWORK"]; gowork != "" {
		// TODO: let this be customizable
		// TODO: report this when verbose flag set
		importDir = filepath.Join(filepath.Dir(gowork), "wharf_port")
	} else {
		log.Fatal("No Go Workspace found; please initialize one using `go work init` and add packages to port")
	}

	return porting.Port(paths, &porting.Config{
		GoEnv:      goenv,
		ImportDir:  importDir,
		Directives: direct.Config,
		BuildTags:  strings.Split(tags, ","),
		Verbose:    verbose,
		DryRun:     dryRun,
	})
}

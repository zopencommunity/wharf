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

	"github.com/mattn/go-isatty"
	"github.com/zosopentools/wharf/internal/direct"
	"github.com/zosopentools/wharf/internal/porting"
	"github.com/zosopentools/wharf/internal/util"
)

var dryRunFlag *bool
var verboseFlag *bool
var testFlag *bool
var configFlag *string
var vcsFlag *bool
var iDirFlag *string
var forceFlag *bool

var tagsFlag *string
var helpFlag *bool

func main() {
	// Parse cmd line flags
	helpFlag = flag.Bool("help", false, "Print help text")
	tagsFlag = flag.String("tags", "", "List of build tags")
	dryRunFlag = flag.Bool("n", false, "Enable dry mode, make suggestions but don't preform changes")
	verboseFlag = flag.Bool("v", false, "Enable verbose output")
	testFlag = flag.Bool("t", false, "Test the package after the porting stage")
	vcsFlag = flag.Bool("q", false, "Clone the package from VCS")
	configFlag = flag.String("config", "", "Config for additional code edits")
	iDirFlag = flag.String("d", "", "Path to store imported modules")
	forceFlag = flag.Bool("f", false, "Force operation even if imported module path exists")
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

	// Verify that we are running in a workspace
	goenv, err := util.GoEnv()
	if err != nil {
		log.Fatal("Unable to read 'go env':", err)
	}

	// Setup import directory
	var importDir string
	if gowork := goenv["GOWORK"]; gowork != "" {
		// TODO: report this when verbose flag set
		if *iDirFlag == "" {
			// TODO: make this relative to the current position
			// so that `go work use` uses a relative position instead of absolute
			importDir = filepath.Join(filepath.Dir(gowork), "wharf_port")
		} else {
			importDir = *iDirFlag
		}

		if *verboseFlag {
			fmt.Println("Import path set to:", importDir)
		}
	} else {
		log.Fatal("No Go Workspace found; please initialize one using `go work init` and add packages to port")
	}

	// Bypass if '-f' flag is included (this is intended for scripts to be able to use if necessary)
	if !*forceFlag {
		_, dstErr := os.Lstat(importDir)
		if dstErr == nil {
			if isatty.IsTerminal(os.Stdin.Fd()) {
				fmt.Printf("WARNING: Import destination already exists (%v)\n", importDir)
				fmt.Println("WARNING: Running Wharf may cause some data to get overridden")
				fmt.Print("Run anyways? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					os.Exit(0)
				}
			} else {
				log.Fatalf("Import destination already exists (%v)\nWill not overwrite. Aborting.", importDir)
			}
		}
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

	tags := strings.Split(*tagsFlag, ",")

	paths := flag.Args()

	if err := main1(paths, tags, *verboseFlag, *dryRunFlag, *vcsFlag, importDir, goenv); err != nil {
		fmt.Println(err.Error())
		fmt.Println("Porting failed due to errors mentioned above")
	} else {
		fmt.Println("Patches applied successfully!")
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

func main1(
	paths []string,
	tags []string,
	verbose bool,
	dryRun bool,
	useVCS bool,
	importDir string,
	goenv map[string]string,
) error {

	return porting.Port(paths, &porting.Config{
		GoEnv:      goenv,
		ImportDir:  importDir,
		Directives: direct.Config,
		BuildTags:  tags,
		Verbose:    verbose,
		DryRun:     dryRun,
		UseVCS:     useVCS,
	})
}

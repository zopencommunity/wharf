// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/util"
)

const shaLen = 7

var (
	// Version contains the application version number. It's set via ldflags
	// when building. (-ldflags="-X 'main.Version=${WHARF_VERSION}'")
	Version = ""

	// CommitSHA contains the SHA of the commit that this application was built
	// against. It's set via ldflags when building.
	// (-ldflags="-X 'main.CommitSHA=$(git rev-parse HEAD)'")
	CommitSHA = ""
)

func main() {
	// Parse cmd line flags
	helpFlag := flag.Bool("help", false, "Print help text")
	tagsFlag := flag.String("tags", "", "List of build tags")
	dryRunFlag := flag.Bool("n", false, "Enable dry mode, make suggestions but don't preform changes")
	verboseFlag := flag.Bool("v", false, "Enable verbose output")
	testFlag := flag.Bool("t", false, "Test the package after the porting stage")
	vcsFlag := flag.Bool("q", false, "Clone the package from VCS")
	configFlag := flag.String("config", "", "Config for additional code edits")
	patchesFlag := flag.Bool("p", false, "Saves patch files to filesystem path")
	iDirFlag := flag.String("d", "", "Path to store imported modules") // TODO: Enable
	forceFlag := flag.Bool("f", false, "Force operation even if imported module path exists")
	versionFlag := flag.Bool("version", false, "Display version information")
	flag.Parse()

	// Turn off log flags
	log.SetFlags(0)

	// If --help is passed
	if *helpFlag {
		fmt.Println(helpText)
		os.Exit(0)
	}

	if *versionFlag {
		if Version == "" {
			if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
				Version = info.Main.Version
			} else {
				Version = "unknown (built from source)"
			}
		}

		if len(CommitSHA) >= shaLen {
			Version += " (" + CommitSHA[:shaLen] + ")"
		}

		fmt.Println(Version)
		os.Exit(0)
	}

	// Verify arg length
	if flag.NArg() < 1 {
		log.Fatal("No package paths provided; see 'wharf --help' for usage")
	}

	// Handle config file argument
	if *configFlag != "" {
		if err := base.LoadInlines(*configFlag); err != nil {
			log.Fatal("Unable to load inlines file", err)
		}
	}

	base.Verbose = *verboseFlag
	base.DryRun = *dryRunFlag
	base.CloneFromVCS = *vcsFlag
	base.GeneratePatches = *patchesFlag

	if base.GeneratePatches && !base.CloneFromVCS {
		log.Fatal("Cannot use -p flag without enabling vcs cloning")
	}

	if len(*tagsFlag) > 0 {
		for _, tag := range strings.Split(*tagsFlag, ",") {
			base.BuildTags[tag] = true
		}
	}

	if len(*iDirFlag) > 0 {
		base.ImportDir = *iDirFlag
	}

	// Bypass if set to force operations (this is intended for scripts to be able to use if necessary)
	if !*forceFlag {
		_, dstErr := os.Lstat(base.ImportDir)
		if dstErr == nil {
			if isatty.IsTerminal(os.Stdin.Fd()) {
				fmt.Printf("WARNING: Import destination already exists: %v\n", base.ImportDir)
				fmt.Println("WARNING: Running Wharf may cause some data to get overridden")
				fmt.Print("Run anyways? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					os.Exit(0)
				}
			} else {
				log.Fatalf("Import destination already exists: %v\nWill not overwrite. Aborting.", base.ImportDir)
			}
		}
	}

	if *verboseFlag {
		fmt.Println("Importing modules to:", base.ImportDir)
	}

	paths := flag.Args()

	if err := main2(paths, !*dryRunFlag); err != nil {
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

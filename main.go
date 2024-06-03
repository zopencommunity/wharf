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
	"runtime/debug"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/porting"
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

	if err := main1(paths, !*dryRunFlag); err != nil {
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
	commit bool,
) error {
	removeWork := true
	if base.GOWORK() == "" {
		log.Fatal("no workspace found; please initialize one using `go work init` and add modules")
	}

	// Setup a private go.work file to make changes to as we work - while keeping the original safe
	wfWork := filepath.Join(filepath.Dir(base.GOWORK()), ".wharf.work")
	if err := util.CopyFile(wfWork, base.GOWORK()); err != nil {
		log.Fatalf("unable to create temporary workspace: %v", err)
	}
	defer func(all *bool) {
		if err := os.Remove(wfWork + ".sum"); err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove: %v: %v", wfWork+".sum", err)
		}
		if *all {
			if err := os.Remove(wfWork); err != nil {
				fmt.Fprintf(os.Stderr, "unable to remove: %v: %v", wfWork, err)
			}
		}
	}(&removeWork)
	if err := os.Setenv("GOWORK", wfWork); err != nil {
		log.Fatalf("unable to update GOWORK: %v", err)
	}

	if err := os.MkdirAll(base.Cache, 0755); err != nil {
		log.Fatalf("unable to create cache at %v: %v", base.Cache, err)
	}
	defer func() {
		if err := os.RemoveAll(base.Cache); err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove cache: %v: %v", base.Cache, err)
		}
	}()

	if perr := porting.Port(paths); perr != nil {
		return perr
	}

	if commit {
		var err error
		backup := base.GOWORK() + ".backup"
		if err = util.CopyFile(backup, base.GOWORK()); err != nil {
			fmt.Fprintf(os.Stderr, "unable to backup workspace to %v: %v\n", backup, err)
		} else if err = util.CopyFile(base.GOWORK(), wfWork); err != nil {
			fmt.Fprintf(os.Stderr, "unable to update workspace: %v\n", wfWork)
		}

		// TODO: this will get displayed on JSON output, which is bad
		if err != nil {
			removeWork = false
			fmt.Println("An error occurred:")
			fmt.Println("\tUnable to replace the current GOWORK file with our copy.")
			fmt.Println("\tTherefore, some patches might not be applied.")
			fmt.Println("\tOur copy is located here:", wfWork)
		}

		fmt.Println("Created workspace backup:", backup)
	}

	return nil
}

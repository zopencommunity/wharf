// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/pkg2"
	"github.com/zosopentools/wharf/internal/tags"
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
		log.Fatal("no package paths provided; see 'wharf --help' for usage")
	}

	// Handle config file argument
	if *configFlag != "" {
		if err := base.LoadInlines(*configFlag); err != nil {
			log.Fatal("unable to load inlines file", err)
		}
	}

	if *patchesFlag && !*vcsFlag {
		log.Fatal("cannot use -p flag without enabling vcs cloning")
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
				fmt.Printf("warning: import destination already exists: %v\n", base.ImportDir)
				fmt.Println("warning: running Wharf may cause some data to get overridden")
				fmt.Print("continue? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					os.Exit(0)
				}
			} else {
				log.Fatalf("error: import destination already exists: %v\n", base.ImportDir)
			}
		}
	}

	if *verboseFlag {
		fmt.Println("importing modules to:", base.ImportDir)
	}

	paths := flag.Args()

	if base.GOWORK() == "" {
		log.Fatalln("no workspace found; please initialize one using `go work init` and add modules")
	}

	// Setup a private go.work file to make changes to as we work - while keeping the original safe
	wfWork := filepath.Join(filepath.Dir(base.GOWORK()), ".wharf.work")
	if err := util.CopyFile(wfWork, base.GOWORK()); err != nil {
		log.Fatalf("unable to create temporary workspace: %v\n", err)
	}
	defer func() {
		if err := os.Remove(wfWork + ".sum"); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("unable to remove: %v: %v\n", wfWork+".sum", err)
		}
	}()

	if err := os.MkdirAll(base.Cache, 0755); err != nil {
		log.Fatalf("unable to create cache at %v: %v\n", base.Cache, err)
	}

	if err := os.Setenv("GOWORK", wfWork); err != nil {
		log.Fatalf("unable to set GOWORK: %v\n", err)
	}

	out, err := main2(paths, false)

	if err != nil {
		log.Println(err.Error())
		log.Fatalln("porting failed due to errors mentioned above")
	}

	fmt.Println("porting successful!")
	fmt.Println("\n--- MODULE CHANGES ---")
	for _, pin := range out.Modules {
		printPin(pin)
	}
	fmt.Println("\n--- PACKAGE CHANGES ---")
	for _, patch := range out.Packages {
		printPatch(patch)
	}

	// Don't apply next steps (patches)
	if *dryRunFlag {
		os.Exit(0)
	}

	failed := false
	madeImportDir := false
	for _, pin := range out.Modules {
		if pin.Imported {
			if !madeImportDir {
				madeImportDir = true
				if err := os.Mkdir(base.ImportDir, 0755); err != nil {
					log.Printf("ERROR: unable to create folder for importing modules: %v: %v", base.ImportDir, err)
					failed = true
					break
				}
			}
			pin.Dir = importFolderName(pin.Path)
			pin.Dir = filepath.Join(base.ImportDir, pin.Dir)
			if err := importModule(pin, *vcsFlag); err != nil {
				failed = true
				log.Printf("ERROR: unable to import module %v@%v: %v\n", pin.Path, pin.Pinned, err)
			}
		}
	}

	if failed {
		log.Fatalln("\nAn error occurred while importing modules.\nPatches will need to be applied manually.")
	}

	for _, patch := range out.Packages {
		if err := applyPatch(patch); err != nil {
			failed = true
			log.Printf("unable to apply patch for %v: %v\n", patch.Path, err)
		}
	}

	if failed {
		log.Fatalln("\nAn error occurred while applying patches.\nPlease apply missing patches manually.")
	}

	backup := base.GOWORK() + ".backup"
	if err = util.CopyFile(backup, base.GOWORK()); err != nil {
		log.Printf("unable to backup workspace to %v: %v\n", backup, err)
	} else if err = util.CopyFile(base.GOWORK(), wfWork); err != nil {
		log.Printf("unable to update workspace: %v\n", wfWork)
	} else {
		fmt.Println("backed up workspace to", backup)
	}

	if err != nil {
		log.Println("An error occurred:")
		log.Println("\tUnable to replace the current GOWORK file with our copy.")
		log.Println("\tTherefore, some patches might not be applied.")
		log.Println("\tOur copy is located here:", wfWork)
	} else {
		if err := os.Remove(wfWork); err != nil {
			log.Printf("unable to remove: %v: %v\n", wfWork, err)
		}
	}

	if err := os.RemoveAll(base.Cache); err != nil {
		log.Printf("unable to remove cache: %v: %v\n", base.Cache, err)
	}

	fmt.Println("patches applied successfully!")

	// TODO: remove
	if *testFlag {
		// Run tests
		fmt.Println("\nRunning tests...")
		if output, err := util.GoTest(paths); err != nil {
			fmt.Println("Tests failed:\n" + output)
		} else {
			fmt.Println("Tests passed!")
		}
	}
}

func printPin(pin base.ModulePin) {
	fmt.Printf("# %v (%v): ", pin.Path, pin.Version)
	if pin.Imported {
		fmt.Println("IMPORTED")
	} else if pin.Pinned != pin.Version {
		fmt.Println("UPDATED TO", pin.Pinned)
	} else {
		fmt.Println("PINNED")
	}
}

func printPatch(patch base.PackagePatch) {
	fmt.Println("#", patch.Path)

	if len(patch.Tags) == 0 {
		fmt.Println("- applied manual patch")
		return
	}

	fmt.Print("- applying tags to match platform(s): ")
	for pidx, pltf := range patch.Tags {
		fmt.Print(pltf)
		if pidx < len(patch.Tags)-1 {
			fmt.Print(", ")
		}
	}
	fmt.Println()

	for _, file := range patch.Files {
		fmt.Printf("- %v:\n", file.Name)
		if file.BaseFile == "" {
			if !file.Build {
				fmt.Printf("\tadded tag '!%v'\n", base.GOOS())
			} else {
				fmt.Printf("\tadded tag '%v'\n", base.GOOS())
			}
		} else {
			fmt.Printf("\tcopied to %v\n", file.BaseFile)

			for _, symbol := range file.Symbols {
				fmt.Printf("\treplaced %v with %v\n", symbol.Original, symbol.New)
			}
		}
	}
}

func importModule(pin base.ModulePin, useVCS bool) error {
	if !pin.Imported {
		return nil
	}

	if useVCS {
		if err := util.CloneModuleFromVCS(
			pin.Dir,
			pin.Path,
			strings.TrimSuffix(pin.Pinned, "+incompatible"),
		); err != nil {
			return err
		}
	} else {
		if err := util.CloneModuleFromCache(pin.Dir, pin.Path); err != nil {
			return err
		}
	}

	if err := util.GoWorkEditDropReplace(pin.Path); err != nil {
		return err
	}

	rel, _ := filepath.Rel(filepath.Dir(base.GOWORK()), pin.Dir)
	// TODO: Go work use fails silently on a missing go.mod file, rerun 'go list' to verify it is now a main module position has changed
	if err := util.GoWorkUse(rel); err != nil {
		return err
	}

	err := util.GoListModMain(pin.Path)
	if err != nil && !pkg2.IsExcludeGoListError(err.Error()) {
		return err
	}

	return nil
}

func applyPatch(patch base.PackagePatch) error {
	patch.Dir, _ = util.GoListPkgDir(patch.Path)

	resolveFilePath := func(file string) string {
		return filepath.Join(patch.Dir, file)
	}

	if patch.Template {
		for _, file := range patch.Files {
			if file.BaseFile != "" {
				if err := util.CopyFile(resolveFilePath(file.Name), file.Cached); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Apply changes to files that were changed
	for _, file := range patch.Files {
		if file.BaseFile != "" {
			// Copy the file from the cache
			if err := util.CopyFile(resolveFilePath(file.Name), file.Cached); err != nil {
				return err
			}

			// Add the file tag
			src, err := util.Format(file.Syntax, pkg2.FileSet)
			if err != nil {
				return err
			}

			src, err = util.AppendTagString(src, base.GOOS(), "", fmt.Sprintf(base.FILE_NOTICE, file.BaseFile))
			if err != nil {
				return err
			}

			err = os.WriteFile(resolveFilePath(file.Name), src, 0744)
			if err != nil {
				return err
			}
		} else if file.Build {
			// Append zos tag
			src, err := util.Format(file.Syntax, pkg2.FileSet)
			if err != nil {
				return err
			}

			src, err = util.AppendTagString(src, base.GOOS(), "||", fmt.Sprintf(base.TAG_NOTICE, base.GOOS()))
			if err != nil {
				return err
			}

			name := file.Name
			cnstr, _ := tags.ParseFileName(name)
			if cnstr != nil {
				name = strings.TrimSuffix(name, ".go") + "_" + base.GOOS() + ".go"
			}

			err = os.WriteFile(resolveFilePath(name), src, 0744)
			if err != nil {
				return err
			}
		} else {
			// Append !zos tag
			src, err := util.Format(file.Syntax, pkg2.FileSet)
			if err != nil {
				return err
			}

			src, err = util.AppendTagString(src, "!"+base.GOOS(), "&&", fmt.Sprintf(base.TAG_NOTICE, "!"+base.GOOS()))
			if err != nil {
				return err
			}

			err = os.WriteFile(resolveFilePath(file.Name), src, 0744)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func importFolderName(importPath string) string {
	segments := strings.Split(importPath, "/")
	base := segments[len(segments)-1]
	decor := ""
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			decor = base
			segments = segments[:len(segments)-1]
		}
	}

	base = segments[len(segments)-1]
	if len(segments) > 1 {
		base = segments[len(segments)-2] + "-" + base
	}

	if decor != "" {
		base = base + "-" + decor
	}

	return base
}

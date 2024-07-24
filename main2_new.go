// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

//go:build wharf_refactor

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/pkg2"
	"github.com/zosopentools/wharf/internal/port2"
	"github.com/zosopentools/wharf/internal/tags"
	"github.com/zosopentools/wharf/internal/util"
)

func main2(
	paths []string,
	commit bool,
) error {
	removeWork := true
	if base.GOWORK() == "" {
		log.Fatalln("no workspace found; please initialize one using `go work init` and add modules")
	}

	// Setup a private go.work file to make changes to as we work - while keeping the original safe
	wfWork := filepath.Join(filepath.Dir(base.GOWORK()), ".wharf.work")
	if err := util.CopyFile(wfWork, base.GOWORK()); err != nil {
		log.Fatalf("unable to create temporary workspace: %v\n", err)
	}
	defer func(all *bool) {
		if err := os.Remove(wfWork + ".sum"); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "unable to remove: %v: %v\n", wfWork+".sum", err)
		}
		if *all {
			if err := os.Remove(wfWork); err != nil {
				fmt.Fprintf(os.Stderr, "unable to remove: %v: %v\n", wfWork, err)
			}
		}
	}(&removeWork)
	if err := os.Setenv("GOWORK", wfWork); err != nil {
		log.Fatalf("unable to update GOWORK: %v\n", err)
	}

	if err := os.MkdirAll(base.Cache, 0755); err != nil {
		log.Fatalf("unable to create cache at %v: %v\n", base.Cache, err)
	}
	defer func() {
		if err := os.RemoveAll(base.Cache); err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove cache: %v: %v\n", base.Cache, err)
		}
	}()

	ctx := port2.NewContext()
	if err := run(paths, ctx); err != nil {
		return err
	}

	pins := ctx.CollectPins()
	patches := ctx.CollectPatches()

	out := base.Output{
		Modules:  pins,
		Packages: patches,
	}

	printJson(out)

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

func run(paths []string, ctx *port2.Context) error {
	firstPass := true
load:
	tree, err := pkg2.List(paths)
	if err != nil {
		return err
	}

	err = tree.Resolve()
	if err != nil {
		return err
	}

	groups := tree.Groups()

	for _, group := range groups {
		for _, pkg := range group {
			handle := ctx.GetHandle(pkg)

			// Sanity checks to make sure stdlib packages aren't altered by us
			if !pkg.FirstLoad && (pkg.Meta.Goroot || pkg.Meta.Standard) && (pkg.Dirty || pkg.DepDirty) {
				panic(fmt.Sprintf("GOROOT package %v changed after first load", pkg))
			}

			handle.Refresh()

			// Mark frozen (GOROOT and pinned golang.org/x/...) packages as exhausted
			if pkg.Meta.Goroot || pkg.Meta.Standard || (pkg2.IsGolangXPkg(pkg) && pkg.Meta.Module.Replace != nil) {
				handle.MarkExhausted()
			}

			if firstPass && handle.HasTypeErrors() {
				// fmt.Printf("%v: needs inspecting\n", handle.GetPackage().Meta.ImportPath)
			}
		}
	}

	firstPass = false

	for i := range groups {
		packages := groups[len(groups)-(i+1)]

		for _, pkg := range packages {

			result, err := ctx.Port(pkg)
			if result == port2.RESULT_ERROR || err != nil {
				fmt.Printf("Package require manual porting: %v\n\t%v\n", pkg.Meta.ImportPath, err.Error())
				return err
			}

			if result == port2.RESULT_RELOAD {
				goto load
			}
		}
	}

	return nil
}

func printPin(pin base.ModulePin) {
	fmt.Printf("%v (%v): ", pin.Path, pin.Version)
	if pin.Imported {
		fmt.Println("IMPORTED")
	} else if pin.Pinned != pin.Version {
		fmt.Println("UPDATED TO", pin.Pinned)
	} else {
		fmt.Println("PINNED")
	}
}

func printPatch(patch base.PackagePatch) {
	if len(patch.Tags) == 0 {
		fmt.Println("Applied manual patch")
		return
	}

	fmt.Print("Applying tags to match platform(s): ")
	for pidx, pltf := range patch.Tags {
		fmt.Print(pltf)
		if pidx < len(patch.Tags)-1 {
			fmt.Print(", ")
		}
	}
	fmt.Println()

	for _, file := range patch.Files {
		fmt.Printf("%v:", file.Name)
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

func printJson(out base.Output) {
	if outstrm, err := json.MarshalIndent(out, "", "\t"); err == nil {
		fmt.Println(string(outstrm))
	} else {
		fmt.Println(err.Error())
	}
}

func applyPin(pin base.ModulePin) error {
	if !pin.Imported {
		return nil
	}

	if base.CloneFromVCS {
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

	// TODO: Go work use fails silently on a missing go.mod file, rerun 'go list' to verify it is now a main module position has changed
	if err := util.GoWorkUse(pin.Path); err != nil {
		return err
	}

	err := util.GoListModMain(pin.Path)
	if err != nil && !pkg2.IsExcludeGoListError(err.Error()) {
		return err
	}

	return nil
}

func applyPatch(patch base.PackagePatch) error {
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

func generatePatchFiles(path string) error {
	// outdir, _ := filepath.Abs(base.GOWORK())
	// outdir = filepath.Dir(outdir)
	// for path := range diffs {
	// 	out := filepath.Join(outdir, filepath.Base(path)+".patch")
	// 	if err := util.GitDiff(path, out); err != nil {
	// 		fmt.Fprintf(os.Stderr, "Unable to produce patch file for repo located at %v: %v", path, err.Error())
	// 	}
	// }
	return nil
}

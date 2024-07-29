// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package main

import (
	"fmt"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/pkg2"
	"github.com/zosopentools/wharf/internal/port2"
	"github.com/zosopentools/wharf/internal/util"
)

func main2(
	paths []string,
	mute bool,
) (*base.Output, error) {
	ctx := port2.NewContext()
	if err := run(paths, ctx, mute); err != nil {
		return nil, err
	}

	out := &base.Output{
		Modules:  ctx.CollectPins(),
		Packages: ctx.CollectPatches(),
	}

	return out, nil
}

func run(paths []string, ctx *port2.Context, mute bool) error {
	firstPass := true
load:
	if err := util.GoModTidy(); err != nil {
		return err
	}

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

			if !mute && firstPass && handle.HasTypeErrors() {
				fmt.Printf("%v: needs inspecting\n", handle.GetPackage().Meta.ImportPath)
			}
		}
	}

	firstPass = false

	for i := range groups {
		packages := groups[len(groups)-(i+1)]

		for _, pkg := range packages {

			result, err := ctx.Port(pkg)
			if result == port2.RESULT_ERROR || err != nil {
				fmt.Printf("package require manual porting: %v\n\t%v\n", pkg.Meta.ImportPath, err.Error())
				return err
			}

			if result == port2.RESULT_RELOAD {
				goto load
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

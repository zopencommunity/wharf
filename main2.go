// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

//go:build !wharf_refactor

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/porting"
	"github.com/zosopentools/wharf/internal/util"
)

func main2(
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

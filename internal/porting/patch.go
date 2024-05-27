// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package porting

import (
	"bytes"
	"fmt"
	"go/parser"
	"os"
	"path/filepath"

	"github.com/zosopentools/wharf/internal/direct"
	"github.com/zosopentools/wharf/internal/packages"
	"github.com/zosopentools/wharf/internal/util"
)

type modulepatch struct {
	original string
	version  string
	dir      string
	action   uint
}

const (
	modUpdated uint = iota
	modLocked
	modImported
)

// Type check each config until one without issues passes
//
// An optional filter can be provided to filter out which errors
// are to be ignored when determining whether a configuration is valid
func filterConfigs(pkg *packages.Package, filter func(packages.TypeError) bool) {
	for pkg.CfgIdx < len(pkg.Configs) {
		if typeCheck(pkg, filter) {
			return
		}
		pkg.CfgIdx++
	}
}

// Apply a package's file directives to the package's source and attempt a build
func applyPackageDirective(pkg *packages.Package, cache string, directives map[string]direct.FileDirective) error {
	pcfg := packages.BuildConfig{
		Platforms: []string{},
		GoFiles:   make([]*packages.GoFile, 0, len(pkg.Configs[0].GoFiles)),
		Override:  make(map[string]*packages.ExtGoFile, len(directives)),
	}

	for name, fh := range directives {
		switch fh.Type {
		case direct.DiffType:
			// Load diff file contents
			diff, err := util.ReadFile(fh.Path)
			if err != nil {
				return fmt.Errorf("unable to read patch file: %w", err)
			}

			cpath := filepath.Join(cache, name)

			// Apply patch
			err = util.Patch(filepath.Join(pkg.Dir, name), cpath, diff)
			if err != nil {
				return fmt.Errorf("unable to apply patch: %w", err)
			}

			// Create AST for file
			syntax, err := parser.ParseFile(pkg.Fset, cpath, nil, parser.AllErrors)
			if err != nil {
				return fmt.Errorf("unable to parse patched file: %w", err)
			}

			overlay := &packages.ExtGoFile{
				Path:   cpath,
				Syntax: syntax,
				Meta:   "internal patch description",
			}
			pcfg.Override[name] = overlay
		default:
			panic("Unknown explicit file handler type")
		}
	}

	// Add unchanged files from base config to the patched config
	// TODO: what if we patch files not from the base config?
	for idx, gofile := range pkg.Configs[0].GoFiles {
		pcfg.GoFiles = append(pcfg.GoFiles, gofile)
		if overlay := pcfg.Override[gofile.Name]; overlay != nil {
			pcfg.Syntax = append(pcfg.Syntax, overlay.Syntax)
		} else {
			pcfg.Syntax = append(pcfg.Syntax, pkg.Configs[0].Syntax[idx])
		}
	}

	pkg.Configs = append(pkg.Configs, pcfg)
	pkg.CfgIdx = len(pkg.Configs) - 1

	if typeCheck(pkg, nil) {
		return nil
	}

	pkg.CfgIdx = len(pkg.Configs)
	return fmt.Errorf("patched package resulted in badly typed package")
}

// File Name -> Import Name -> Symbol Name -> Directive
type fileImportEdits map[string]map[string]map[string]direct.ExportDirective

// Apply export directives to a package based on the
func applyExportDirective(pkg *packages.Package, cache string, fiEdits fileImportEdits) error {
	ccfg := pkg.Configs[pkg.CfgIdx]
	pcfg := packages.BuildConfig{
		Platforms: []string{packages.Goos},
		GoFiles:   make([]*packages.GoFile, 0, len(ccfg.GoFiles)),
		Override:  make(map[string]*packages.ExtGoFile, len(fiEdits)),
	}

	// Apply the changes and make copies of files, store files in cache
	for idx := range ccfg.GoFiles {
		gofile := ccfg.GoFiles[idx]

		iEdits := fiEdits[gofile.Name]
		if iEdits == nil {
			pcfg.GoFiles = append(pcfg.GoFiles, gofile)
			pcfg.Syntax = append(pcfg.Syntax, ccfg.Syntax[idx])
			continue
		}

		file, err := os.ReadFile(filepath.Join(pkg.Dir, gofile.Name))
		if err != nil {
			// TODO: better info
			return fmt.Errorf("unable to read file for custom import replacement: %w", err)
		}

		cpath := filepath.Join(cache, gofile.Name)

		for iname, sEdits := range iEdits {
			for sname, ed := range sEdits {
				var repstr string
				switch ed.Type {
				case "EXPORT":
					repstr = iname + "." + ed.Replace
				case "CONST":
					repstr = ed.Replace
				default:
					panic("unknown export directive type")
				}
				// TODO: handle cases where import name is "."
				file = bytes.ReplaceAll(file, ([]byte)(iname+"."+sname), ([]byte)(repstr))
			}
		}

		// Create AST for file
		syntax, err := parser.ParseFile(pkg.Fset, cpath, file, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("unable to apply custom import patch: unable to parse patched file: %w", err)
		}

		err = os.WriteFile(cpath, file, 0740)
		if err != nil {
			// TODO: better inf
			return fmt.Errorf("unable to apply custom import replacement: %w", err)
		}

		pcfg.GoFiles = append(pcfg.GoFiles, gofile)
		pcfg.Syntax = append(pcfg.Syntax, syntax)
		pcfg.Override[gofile.Name] = &packages.ExtGoFile{
			Path:   cpath,
			Syntax: syntax,
			Meta:   iEdits,
		}
	}

	pkg.Configs = append(pkg.Configs, pcfg)
	pkg.CfgIdx = len(pkg.Configs) - 1

	// Verify the config
	if typeCheck(pkg, nil) {
		return nil
	}

	pkg.CfgIdx = len(pkg.Configs)

	return fmt.Errorf("patched export directives resulted in badly typed package")
}

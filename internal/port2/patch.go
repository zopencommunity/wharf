// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package port2

import (
	"bytes"
	"fmt"
	"go/parser"
	"os"
	"path/filepath"
	"strings"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/pkg2"
	"github.com/zosopentools/wharf/internal/tags"
)

type workEdit struct {
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

// Apply a package's file directives to the package's source and attempt a build
// func (c *Controller) applyPackageDirective(pkg *pkg2.Package, cache string, directives map[string]base.FileInline) error {
// 	state := c.states[pkg]
// 	pcfg := pkg2.BuildConfig{
// 		Platforms: []string{},
// 		Files:     make([]*pkg2.GoFile, 0, len(pkg.Builds[0].Files)),
// 	}

// 	for name, fh := range directives {
// 		switch fh.Type {
// 		case base.InlineDiffSym:
// 			gofile := pkg.Files[name]
// 			// Load diff file contents
// 			diff, err := util.ReadFile(fh.Path)
// 			if err != nil {
// 				return fmt.Errorf("unable to read patch file: %w", err)
// 			}

// 			cpath := filepath.Join(cache, name)

// 			// Apply patch
// 			err = util.Patch(filepath.Join(pkg.Meta.Dir, name), cpath, diff)
// 			if err != nil {
// 				return fmt.Errorf("unable to apply patch: %w", err)
// 			}

// 			// Create AST for file
// 			syntax, err := parser.ParseFile(pkg.FileSet, cpath, nil, parser.AllErrors)
// 			if err != nil {
// 				return fmt.Errorf("unable to parse patched file: %w", err)
// 			}

// 			repl := &pkg2.GoFile{
// 				Name:    fmt.Sprintf("%v_%v.go", strings.TrimSuffix(gofile.Name, ".go"), base.GOOS()),
// 				Path:    cpath,
// 				Cgo:     gofile.Cgo,
// 				Syntax:  syntax,
// 				Imports: gofile.Imports,
// 				Replaced: &pkg2.ReplacedFile{
// 					File:   gofile,
// 					Reason: "internal patch description",
// 				},
// 			}
// 			pcfg.Files = append(pcfg.Files, repl)
// 			pcfg.Syntax = append(pcfg.Syntax, syntax)
// 		default:
// 			panic("Unknown explicit file handler type")
// 		}
// 	}

// 	// Add unchanged files from base config to the patched config
// 	// TODO: what if we patch files not from the base config?
// 	for idx, gofile := range pkg.Files {
// 		pcfg.Files = append(pcfg.Files, gofile)
// 		if overlay := pcfg.Override[gofile.Name]; overlay != nil {
// 			pcfg.Syntax = append(pcfg.Syntax, overlay.Syntax)
// 		} else {
// 			pcfg.Syntax = append(pcfg.Syntax, pkg.Builds[0].Syntax[idx])
// 		}
// 	}

// 	pkg.Builds = append(pkg.Builds, pcfg)
// 	state.cfi = len(pkg.Builds) - 1

// 	if c.validate(pkg, nil) {
// 		return nil
// 	}

// 	state.cfi = len(pkg.Builds)
// 	return fmt.Errorf("patched package resulted in badly typed package")
// }

// File Name -> Import Name -> Symbol Name -> Directive
type fileImportEdits map[string]map[string]map[string]base.ExportInline

// Apply export directives to a package based on the
func (c *Controller) applyExportDirective(pkg *pkg2.Package, cache string, fiEdits fileImportEdits) error {
	state := c.states[pkg]
	ccfg := pkg.Builds[state.cfi]
	pcfg := pkg2.BuildConfig{
		Platforms: []string{base.GOOS()},
		Files:     make([]*pkg2.GoFile, 0, len(ccfg.Files)),
	}

	// Apply the changes and make copies of files, store files in cache
	for idx := range ccfg.Files {
		gofile := ccfg.Files[idx]

		iEdits := fiEdits[gofile.Name]
		if iEdits == nil {
			pcfg.Files = append(pcfg.Files, gofile)
			pcfg.Syntax = append(pcfg.Syntax, ccfg.Syntax[idx])
			continue
		}

		file, err := os.ReadFile(filepath.Join(pkg.Meta.Dir, gofile.Name))
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
		syntax, err := parser.ParseFile(pkg2.FileSet, cpath, file, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("unable to apply custom import patch: unable to parse patched file: %w", err)
		}

		err = os.WriteFile(cpath, file, 0740)
		if err != nil {
			// TODO: better inf
			return fmt.Errorf("unable to apply custom import replacement: %w", err)
		}

		repl := &pkg2.GoFile{
			Name:    fmt.Sprintf("%v_%v.go", strings.TrimSuffix(gofile.Name, ".go"), base.GOOS()),
			Path:    cpath,
			Cgo:     gofile.Cgo,
			Syntax:  syntax,
			Tags:    tags.Supported{},
			Imports: gofile.Imports,
			Replaced: &pkg2.ReplacedFile{
				File:   gofile,
				Reason: iEdits,
			},
		}

		pcfg.Files = append(pcfg.Files, repl)
		pcfg.Syntax = append(pcfg.Syntax, syntax)
	}

	pkg.Builds = append(pkg.Builds, pcfg)
	state.cfi = len(pkg.Builds) - 1

	// Verify the config
	if c.validate(pkg, nil) {
		return nil
	}

	state.cfi = len(pkg.Builds)

	return fmt.Errorf("patched export directives resulted in badly typed package")
}

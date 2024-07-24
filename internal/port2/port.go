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
	"github.com/zosopentools/wharf/internal/util"
)

type PortingError struct {
	Package *pkg2.Package
	Error   error
}

type Result uint8

const (
	RESULT_CONTINUE Result = iota
	RESULT_SKIPPED
	RESULT_RELOAD
	RESULT_PATCHED
	RESULT_ERROR
)

func (ctx *Context) Port(pkg *pkg2.Package) (Result, error) {
	handle := ctx.handles[pkg]
	if handle == nil {
		panic("pkg not registered")
	}

	if !handle.included || handle.exhausted {
		return RESULT_SKIPPED, nil
	}

	if handle.patched {
		if handle.incomplete {
			panic("trying to port package that already has patch associated with it")
		}
		return RESULT_CONTINUE, nil
	}

	if !handle.built {
		handle.types, handle.errs = handle.typeCheck(handle.buildIdx, defaultTypeConfig())
	}

	if len(handle.errs) == 0 && !handle.incomplete {
		if handle.buildIdx > 0 {
			handle.patched = true
		}
		return RESULT_CONTINUE, nil
	}

	if !pkg.Meta.Module.Main {
		if changed, err := ctx.pin(pkg.Meta.Module); err != nil {
			return RESULT_ERROR, err
		} else if changed {
			return RESULT_RELOAD, nil
		}
	}

	baseId := handle.buildIdx
	err := handle.port()
	if err != nil {
		return RESULT_ERROR, err
	}

	if baseId != handle.buildIdx {
		pkg.MarkModified()
	}

	if handle.patched {
		if !pkg.Meta.Module.Main {
			pin := ctx.pins[pkg.Meta.Module.Path]
			pin.imported = true
			ctx.pins[pkg.Meta.Module.Path] = pin
		}
		return RESULT_PATCHED, nil
	}

	return RESULT_CONTINUE, err
}

func (ctx *Context) pin(module *pkg2.Module) (bool, error) {
	var err error
	pin := ctx.pins[module.Path]

	// First lock the version the module will use (using the following process):
	//
	// 1. If the module can be updated we try locking it to the updated version
	// 2. If we already tried the updated version then lock it to the original version determined by MVS
	if module.Replace == nil || (pin.isPinned() && pin.pinTo != pin.version) {
		pinTo := module.Version

		if !pin.isPinned() {
			pinTo, err = util.GoListModUpdate(module.Path)
			if err != nil && !pkg2.IsExcludeGoListError(err.Error()) {
				return false, err
			}
		}

		if err = util.GoWorkEditReplaceVersion(
			module.Path,
			pinTo,
		); err != nil {
			return false, err
		}

		oldVer := pin.pinTo
		if oldVer == "" {
			oldVer = module.Version
		}

		ctx.pins[module.Path] = versionPin{
			version: module.Version,
			pinTo:   pinTo,
		}

		if oldVer != pinTo {
			return true, err
		}

	}

	return false, nil
}

// Run the build + port process on a package
func (handle *Handle) port() error {
	pkg := handle.pkg

	// If this is the first time checking this package verify
	// that it has errors before we begin our investigation
	imports := make(map[*pkg2.Package]bool, 0)
	needTag := handle.incomplete
	handle.incomplete = false

	var illList []pkg2.TypeError
	for _, err := range handle.errs {
		if iname, ok := err.Reason.(pkg2.TCBadImportName); ok {
			ipkg := pkg.LookupImport(iname.PkgName, err.Err.Fset.Position(err.Err.Pos).Filename)

			if ipkg == nil {
				panic("bad import on unknown package")
			}
			if handle.ctx.handles[ipkg].exhausted {
				needTag = true
			} else {
				imports[ipkg] = true
			}

		} else if _, ok := err.Reason.(pkg2.TCBadName); ok {
			needTag = true
		} else {
			illList = append(illList, err)
		}
	}

	// If we saw no errors, move on
	if !needTag && len(imports) == 0 {
		handle.valid = true
		return nil
	}

	// Never try porting a package with unknown type errors
	if len(illList) > 0 {
		return fmt.Errorf("unknown type error(s) occurred in %v: %v", pkg.Meta.ImportPath, illList)
	}

	// Have to do tagging
	if needTag {
		// Use the error filter to rebuild the import list whenever we decide to use a new config
		build := handle.buildIdx + 1
		for build < len(pkg.Builds) {
			pkg.LoadSyntax(build)
			typed, errs := handle.typeCheck(build, defaultTypeConfig())
			imports = make(map[*pkg2.Package]bool)

			satisfied := true
			for _, err := range errs {
				if iname, ok := err.Reason.(pkg2.TCBadImportName); ok {
					ipkg := pkg.LookupImport(iname.PkgName, err.Err.Fset.Position(err.Err.Pos).Filename)

					if ipkg == nil {
						panic("bad import on unknown package")
					}
					imports[ipkg] = true
				} else if !err.Err.Soft {
					satisfied = false
					break
				}
			}

			if satisfied {
				handle.buildIdx = build
				handle.types = typed
				handle.errs = errs

				if handle.validate() {
					break
				}
			}
			build++
		}

		if build >= len(pkg.Builds) {
			handle.MarkExhausted()
			return fmt.Errorf("unable to find a valid config")
		}
	}

	if len(imports) == 0 {
		handle.patched = true
		return nil
	}

	// Keep track if any imports are portable
	canPortImports := false
	for ipkg := range imports {
		ih := handle.ctx.handles[ipkg]
		ih.incomplete = true
		ih.included = true

		if ih.patched {
			panic("package is claimed to be patchable but has bad parent")
		}

		if !ih.exhausted {
			canPortImports = true
		}
	}

	// Try porting imports first if possible
	if canPortImports {
		return nil
	}

	// Try retagging to remove the bad imports
	fiEdits := make(fileImportEdits)
	fiBuild := handle.buildIdx
	build := handle.buildIdx + 1
	for build < len(pkg.Builds) {
		if fiEdits == nil {
			fiEdits = make(fileImportEdits)
			fiBuild = build
		}

		pkg.LoadSyntax(build)
		typed, errs := handle.typeCheck(build, defaultTypeConfig())

		satisfied := true
		for _, err := range errs {
			if info, ok := err.Reason.(pkg2.TCBadImportName); ok {
				file := err.Err.Fset.Position(err.Err.Pos).Filename
				ipkg := pkg.LookupImport(info.PkgName, file)
				if ipkg == nil {
					panic("bad import on unknown package")
				}

				directives := base.Inlines[ipkg.Meta.ImportPath]
				if directives != nil && directives.Exports != nil {
					ed, ok := directives.Exports[info.Name.Name]
					if ok {
						if fiEdits[file] == nil {
							fiEdits[file] = make(map[string]map[string]base.ExportInline)
							fiEdits[file][info.PkgName] = make(map[string]base.ExportInline)
						} else if fiEdits[file][info.PkgName] == nil {
							fiEdits[file][info.PkgName] = make(map[string]base.ExportInline)
						}

						fiEdits[file][info.PkgName][info.Name.Name] = ed
					}
				}
			}

			if !err.Err.Soft {
				if fiBuild == build {
					fiEdits = nil
				}

				satisfied = false
				break
			}
		}

		if satisfied {
			handle.buildIdx = build
			handle.types = typed
			handle.errs = errs

			if handle.validate() {
				break
			}
		}
		build++
	}

	if build < len(pkg.Builds) {
		handle.patched = true
		return nil
	} else if fiEdits == nil {
		return fmt.Errorf("unable to find a valid config")
	}

	pkgCacheDir := filepath.Join(base.Cache, pkg.Meta.ImportPath)
	if err := os.MkdirAll(pkgCacheDir, 0740); err != nil {
		return fmt.Errorf("unable to create cache directory for package: %w", err)
	}

	// Didn't find a working config, therefore we try to use export directives
	err := handle.applyExportDirective(fiBuild, pkgCacheDir, fiEdits)
	if err != nil {
		return err
	}

	typed, errs := handle.typeCheck(len(pkg.Builds)-1, defaultTypeConfig())
	if len(errs) > 0 {
		handle.MarkExhausted()
		return fmt.Errorf("inline edits resulted in a bad config")
	}

	handle.types = typed
	handle.buildIdx = fiBuild

	// Verify the config
	if handle.validate() {
		handle.patched = true
		return nil
	}

	// We couldn't find a working config, so we try and see if we can fix the package using explicit handlers
	//
	// TODO: RE-IMPLEMENT THIS
	// if handler := base.Inlines[pkg.Meta.ImportPath]; handler != nil && handler.Files != nil {
	// 	err := c.applyPackageDirective(pkg, pkgCacheDir, handler.Files)
	// 	// TODO: report on diff failing
	// 	if err == nil {
	// 		state.ps = psPatched
	// 		return nil
	// 	}
	// }

	handle.MarkExhausted()
	return fmt.Errorf("no applicable options available to port package %v", pkg.Meta.ImportPath)
}

func (handle *Handle) validate() bool {
	pkg := handle.pkg
	for _, parent := range pkg.Parents {
		ph := handle.ctx.handles[parent]
		tcfg := defaultTypeConfig()
		// TODO: run through parents in order of level (prevent potential mixmatched type errors)
		// TODO: update parents types on success (in case of potential type conflict later on)
		_, errs := ph.typeCheck(ph.buildIdx, tcfg)

		for _, err := range errs {
			// We only care about errors from local imports
			if info, ok := err.Reason.(pkg2.TCBadImportName); ok {
				ipath, ok := parent.Files[err.Err.Fset.Position(err.Err.Pos).Filename].Imports[info.PkgName]
				if !ok {
					if backup := pkg2.BackupNameLookup(info.PkgName); backup != nil {
						ipath = backup.Meta.ImportPath
					} else {
						panic("bad import error but no known name in lookup")
					}
				}

				// If we have a match then that means the parents failed because of
				// of the package under test, therefore we have a bad build
				if pkg.Meta.ImportPath == ipath {
					return false
				}
			} else if !err.Err.Soft {
				// TODO: handle gracefully (allow cleanup)
				// panic(
				// 	fmt.Sprintf(
				// 		"unsanitized parent package: %v for %v has error %v",
				// 		parent.ImportPath, pkg.Meta.ImportPath,
				// 		err.Err.Error(),
				// 	),
				// )
			}
		}
	}

	return true
}

// File Name -> Import Name -> Symbol Name -> Directive
type fileImportEdits map[string]map[string]map[string]base.ExportInline

// Apply export directives to a package based on the
func (handle *Handle) applyExportDirective(build int, cache string, fiEdits fileImportEdits) error {
	pkg := handle.pkg
	ccfg := pkg.Builds[build]
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
				case base.InlineExportSym:
					repstr = iname + "." + ed.Replace
				case base.InlineConstSym:
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
	return nil
}

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

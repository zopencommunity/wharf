// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package port2

import (
	"fmt"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/pkg2"
)

type Context struct {
	handles map[*pkg2.Package]*Handle
	pins    map[string]versionPin
}

type versionPin struct {
	version  string
	pinTo    string
	imported bool
}

func (pin versionPin) isPinned() bool {
	return pin.pinTo != ""
}

func NewContext() *Context {
	return &Context{
		handles: make(map[*pkg2.Package]*Handle),
		pins:    make(map[string]versionPin),
	}
}

func (ctx *Context) GetHandle(pkg *pkg2.Package) *Handle {
	if ctx.handles[pkg] == nil {
		ctx.handles[pkg] = &Handle{
			pkg: pkg,
			ctx: ctx,
		}
	}
	return ctx.handles[pkg]
}

func (ctx *Context) CollectPins() []base.ModulePin {
	pins := make([]base.ModulePin, 0, len(ctx.pins))
	for path, pin := range ctx.pins {
		pins = append(pins, base.ModulePin{
			Path:     path,
			Version:  pin.version,
			Pinned:   pin.pinTo,
			Imported: pin.imported,
		})
	}
	return pins
}

func (ctx *Context) CollectPatches() []base.PackagePatch {
	patches := make([]base.PackagePatch, 0, 20)
	for pkg, handle := range ctx.handles {
		if !handle.patched {
			continue
		}

		files := make([]base.FilePatch, 0, len(pkg.Builds[handle.buildIdx].Files))

		// Mark the files that were active in the default config
		defaultFiles := make(map[*pkg2.GoFile]bool)
		for _, gofile := range pkg.Builds[0].Files {
			defaultFiles[gofile] = true
		}

		// Apply changes to files that were changed
		for _, gofile := range pkg.Builds[handle.buildIdx].Files {
			if defaultFiles[gofile] {
				delete(defaultFiles, gofile)
				continue
			}
			var fileAction base.FilePatch
			fileAction.Name = gofile.Name
			fileAction.Build = true
			fileAction.Cached = gofile.Path
			fileAction.Syntax = gofile.Syntax

			if gofile.Replaced != nil {
				repl := gofile.Replaced.File
				fileAction.BaseFile = repl.Name

				for iname, symbols := range gofile.Replaced.Reason.(map[string]map[string]base.ExportInline) {
					for symname, ed := range symbols {
						var repstr string
						switch ed.Type {
						case base.InlineExportSym:
							repstr = iname + "." + ed.Replace
						case base.InlineConstSym:
							repstr = ed.Replace
						default:
							panic("unknown export directive type")
						}
						fileAction.Symbols = append(fileAction.Symbols, base.SymbolRepl{
							Original: fmt.Sprintf("%v.%v", iname, symname),
							New:      repstr,
						})
					}
				}
			}

			files = append(files, fileAction)
		}

		// Any files that we have left and aren't seen in the current config, we tag to exclude
		for gofile := range defaultFiles {
			var fileAction base.FilePatch
			fileAction.Name = gofile.Name
			fileAction.Build = false
			files = append(files, fileAction)
		}

		patches = append(patches, base.PackagePatch{
			Path:   pkg.Meta.ImportPath,
			Module: pkg.Meta.Module.Path,
			Tags:   pkg.Builds[handle.buildIdx].Platforms,
			Files:  files,
		})

	}

	return patches
}

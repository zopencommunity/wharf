// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package port2

import (
	"fmt"
	"go/types"

	"github.com/zosopentools/wharf/internal/pkg2"
)

type Stage uint8

type Handle struct {
	pkg *pkg2.Package
	ctx *Context

	types *types.Package
	errs  []pkg2.TypeError

	buildIdx int

	// Package has valid and complete type data for the current selected build
	built      bool
	incomplete bool
	included   bool

	exhausted bool
	patched   bool
	valid     bool
}

func (handle *Handle) MarkIncomplete() {
	handle.incomplete = true
}

func (handle *Handle) MarkIncluded() {
	handle.included = true
}

func (handle *Handle) MarkExhausted() {
	handle.exhausted = true
}

func (handle *Handle) GetPackage() *pkg2.Package {
	return handle.pkg
}

func (handle *Handle) HasTypeErrors() bool {
	return len(handle.errs) > 0
}

func (handle *Handle) Refresh() {
	pkg := handle.pkg
	if handle.buildIdx > 0 && pkg.Dirty {
		handle.panic("pinned package was marked dirty")
	}
	handle.incomplete = false
	handle.included = handle.included || pkg.Included

	if pkg.Dirty || pkg.DepDirty {
		if len(pkg.Builds[handle.buildIdx].Syntax) == 0 {
			handle.types = types.NewPackage(pkg.Meta.ImportPath, pkg.Meta.Name)
			handle.types.MarkComplete()
		} else {
			tcfg := &types.Config{
				IgnoreFuncBodies: !pkg.Included || pkg2.IsStdlibPkg(pkg),
				FakeImportC:      true,
			}
			handle.types, handle.errs = handle.typeCheck(handle.buildIdx, tcfg)
		}
		handle.built = true
	}
}

func (handle *Handle) typeCheck(build int, cfg *types.Config) (typed *types.Package, errs []pkg2.TypeError) {
	cfg.Error = func(err error) {
		errs = append(errs, pkg2.NewTypeCheckError(err.(types.Error)))
	}

	cfg.Importer = (importer)(func(path string) (*types.Package, error) {
		if path == pkg2.UNSAFE_PACKAGE_NAME {
			return types.Unsafe, nil
		}

		ipkg := handle.pkg.Imports[path]
		if ipkg == nil {
			handle.panic(fmt.Sprintf("unknown imported package %v requested during type check", path))
		}

		ih := handle.ctx.handles[ipkg]
		if ih == nil {
			handle.panic(fmt.Sprintf("imported package %v with uninitialized state found during type check", path))
		}

		if ih.types == nil {
			handle.panic(fmt.Sprintf("imported package %v with unintialized types object found during type check", path))
		}

		return ih.types, nil
	})

	typed, _ = cfg.Check(handle.pkg.Meta.ImportPath, pkg2.FileSet, handle.pkg.Builds[build].Syntax, nil)
	return
}

func (handle *Handle) panic(msg string) {
	panic(fmt.Sprintf("%v: %v", handle.pkg.Meta.ImportPath, msg))
}

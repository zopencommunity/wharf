// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package packages

import (
	"fmt"
	"go/types"
)

// Build the package
//
// The package will be built according to the specified config (Package.Configs[Package.CfgIdx])
//
// Pass in a non-nil importer function to override import calls
// Pass in a non-nil handleErr function to handle error calls as they appear
func (pkg *Package) Build(importer Importer, handleErr func(err TypeError)) (*types.Package, []TypeError) {
	var errs []TypeError

	imptF := iImporter(func(path string) (*types.Package, error) {
		if path == "unsafe" {
			return types.Unsafe, nil
		}

		if importer != nil {
			ipkg, err := importer(path)
			if ipkg != nil || err != nil {
				return ipkg, err
			}
		}

		ipkg := pkg.Imports[path]
		if ipkg == nil {
			ipkg = globalpkgs[path]
		}
		// If it still fails, something wrong occurred
		if ipkg == nil {
			return nil, fmt.Errorf("unknown import found: %v imported by: %v", path, pkg.ImportPath)
		}

		return ipkg.Types, nil
	})

	errF := func(err error) {
		err2 := TypeError{
			Err:    err.(types.Error),
			Reason: parseTypeErrorReason(err.(types.Error)),
		}
		errs = append(errs, err2)
		if handleErr != nil {
			handleErr(err2)
		}
	}

	tcfg := &types.Config{
		Importer:    imptF,
		Error:       errF,
		FakeImportC: true,
	}

	if pkg.CfgIdx >= len(pkg.Configs){
			pkg.CfgIdx = 0
	}
	ptype, _ := tcfg.Check(pkg.ImportPath, pkg.Fset, pkg.Configs[pkg.CfgIdx].Syntax, nil)
	return ptype, errs
}

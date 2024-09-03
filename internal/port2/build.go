// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.

package port2

import (
	"go/types"
)

type importer func(path string) (*types.Package, error)

func (f importer) Import(path string) (*types.Package, error) {
	return f(path)
}

func defaultTypeConfig() *types.Config {
	return &types.Config{FakeImportC: true}
}

package port2

import "go/types"

type importer func(path string) (*types.Package, error)

func (f importer) Import(path string) (*types.Package, error) {
	return f(path)
}

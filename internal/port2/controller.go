package port2

import "github.com/zosopentools/wharf/internal/pkg2"

type Controller struct {
	paths  []string
	tree   pkg2.ImportTree
	states map[*pkg2.Package]*state

	patchable map[*pkg2.Package]bool
	workspace map[string]*workEdit

	// Errors that occurred during porting of a package
	Errors []error

	// Control
	treeIsDirty bool

	// Ensure controller is only ran once
	complete bool

	// Metrics
	loadCount uint
}

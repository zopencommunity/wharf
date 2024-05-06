// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package porting

import (
	"github.com/zosopentools/wharf/internal/packages"
)

// pstate states
const (
	// Starting state
	stateUnknown uint8 = iota
	// Package was marked as incomplete by a parent
	stateBrokeParent
	// Package has gone through export/local tagging
	statePortingDependencies

	// Package cannot be editted further
	stateExhausted
	// Package has patches to be applied
	statePatched
	// Package already is valid
	stateValid
)

// Initialize the state of a package
//
// Initialize the state object here, also perform extra checking for
// if the package already exists in the workspace.
//
// For standard library and golang.org/x/... packages we perform additional
// processing here to prevent having to call port on them directly if possible
func initProcFlags(pkg *packages.Package) {
	if pkg.Goroot || (isGolangX(pkg.Module) && pkg.Module.Replace != nil) {
		pkg.ExtFlags = stateExhausted
	}
}

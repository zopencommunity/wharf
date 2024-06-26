// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package port2

import (
	"go/types"

	"github.com/zosopentools/wharf/internal/pkg2"
)

type pstate uint8

type state struct {
	types *types.Package
	errs  []pkg2.TypeError
	cfi   int
	ps    pstate
}

// pstate states
const (
	// Starting state
	psUnknown pstate = iota
	// Package has been built at least once
	psBuilt
	// Package was marked as incomplete by a parent
	psBrokeParent
	// Package has gone through export/local tagging
	psPortingDependencies

	// Package cannot be editted further
	psExhausted
	// Package has patches to be applied
	psPatched
	// Package already is valid
	psValid
)

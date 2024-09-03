// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.

package port2

import "fmt"

type PatchError struct {
	PkgPath string

	Reason string

	Suggestion string

	// TODO: Add patch trace
}

func (e PatchError) Error() string {
	return fmt.Sprintf("cannot patch %q because %v", e.PkgPath, e.Reason)
}

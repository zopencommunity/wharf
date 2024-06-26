// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

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

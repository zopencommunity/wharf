// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.

package tags

import (
	"testing"
)

// // // // // // // // //
// AND EXPRESSION CASES //
// // // // // // // // //

func TestNeverAndX(t *testing.T) {
	var result Constraint
	left := Ignored{}

	// Never AND Never IS Never
	result = handleAndExpr(left, Ignored{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Never AND Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never AND Always IS Never
	result = handleAndExpr(left, All{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Never AND Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never AND Goos IS Never
	result = handleAndExpr(left, Supported{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Never AND Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never AND When IS Never
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"zos": true,
			},
		),
		"zos",
	)
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Never AND When IS NOT %[1]v (%[1]v)", result)
		return
	}
}

func TestGoosAndX(t *testing.T) {
	var result Constraint
	left := Supported{}

	// Goos AND Never IS Never
	result = handleAndExpr(left, Ignored{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Goos AND Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos AND Always IS Goos
	result = handleAndExpr(left, All{}, "zos")
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos AND Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos AND Goos IS Goos
	result = handleAndExpr(left, Supported{}, "zos")
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos AND Goos IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos AND When IS Never IF When NOT CONTAINS Goos
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"linux":  true,
				"darwin": true,
			},
		),
		"zos",
	)
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Goos AND When (w/o Goos) IS NOT %[1]T (%[1]v)", result)
		return
	}

	// Goos AND When IS Goos IF When CONTAINS Goos
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"linux":  true,
				"darwin": true,
				"zos":    true,
			},
		),
		"zos",
	)
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos AND When (w/ Goos) IS NOT %[1]T (%[1]v)", result)
		return
	}
}

func TestAlwaysAndX(t *testing.T) {
	var result Constraint
	left := All{}

	// Always AND Never IS Never
	result = handleAndExpr(left, Ignored{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Always AND Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always AND Always IS Always
	result = handleAndExpr(left, All{}, "zos")
	if _, ok := result.(All); !ok {
		t.Errorf("Always AND Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always AND Goos IS Goos
	result = handleAndExpr(left, Supported{}, "zos")
	if _, ok := result.(Supported); !ok {
		t.Errorf("Always AND Goos IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always AND When IS When
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"linux":  true,
				"darwin": true,
			},
		),
		"zos",
	)
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] && rWhen["darwin"] && len(rWhen) == 2) {
			t.Errorf("Always AND When - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("Always AND When (w/o Goos) IS NOT %[1]T", result)
		return
	}
}

func TestWhenAndX(t *testing.T) {
	var result Constraint
	left := Platforms(
		map[string]bool{
			"linux":  true,
			"darwin": true,
		},
	)

	// When AND Never IS Never
	result = handleAndExpr(left, Ignored{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("When AND Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// When AND Always IS When
	result = handleAndExpr(left, All{}, "zos")
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] && rWhen["darwin"] && len(rWhen) == 2) {
			t.Errorf("When AND Always - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("When AND Always IS NOT %[1]T", result)
		return
	}

	// When AND Goos IS Never IF When NOT CONTAINS Goos
	result = handleAndExpr(left, Supported{}, "zos")
	if _, ok := result.(Ignored); !ok {
		t.Errorf("When AND Goos (w/o Goos) IS NOT %[1]v (%[1]v)", result)
		return
	}

	// When AND Goos IS Goos IF When CONTAINS Goos
	left["zos"] = true
	result = handleAndExpr(left, Supported{}, "zos")
	if _, ok := result.(Supported); !ok {
		t.Errorf("When AND Goos (w/ Goos) IS NOT %[1]T (%[1]v)", result)
		return
	}

	// When AND When IS Never IF INTERSECTION EMPTY
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"freebsd": true,
				"openbsd": true,
			},
		),
		"zos",
	)
	if _, ok := result.(Ignored); !ok {
		t.Errorf("When AND When (EMPTY) IS NOT %[1]T (%[1]v)", result)
		return
	}

	// When AND When IS When IF INTERSECTION NOT EMPTY
	result = handleAndExpr(
		left,
		Platforms(
			map[string]bool{
				"linux": true,
				"zos":   true,
			},
		),
		"zos",
	)
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] && rWhen["zos"] && len(rWhen) == 2) {
			t.Errorf("When AND When (NON EMPTY) - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("When AND When (NON EMPTY) IS NOT %[1]T", result)
		return
	}

}

// // // // // // // // //
// OR EXPRESSIONS CASES //
// // // // // // // // //

func TestNeverOrX(t *testing.T) {
	var result Constraint
	left := Ignored{}

	// Never OR Never IS Never
	result = handleOrExpr(left, Ignored{})
	if _, ok := result.(Ignored); !ok {
		t.Errorf("Never OR Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never OR Always IS Always
	result = handleOrExpr(left, All{})
	if _, ok := result.(All); !ok {
		t.Errorf("Never OR Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never OR Goos IS Goos
	result = handleOrExpr(left, Supported{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("Never OR Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Never OR When IS When
	result = handleOrExpr(
		left,
		Platforms(
			map[string]bool{
				"linux": true,
				"zos":   true,
			},
		),
	)
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] && rWhen["zos"] && len(rWhen) == 2) {
			t.Errorf("Never OR When - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("Never OR When IS NOT %[1]T", result)
		return
	}
}

func TestGoosOrX(t *testing.T) {
	var result Constraint
	left := Supported{}

	// Goos OR Never IS Goos
	result = handleOrExpr(left, Ignored{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos OR Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos OR Always IS Goos
	result = handleOrExpr(left, All{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos OR Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos OR Goos IS Goos
	result = handleOrExpr(left, Supported{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos OR Goos IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Goos OR Goos IS Goos
	result = handleOrExpr(
		left,
		Platforms(
			map[string]bool{
				"linux":  true,
				"darwin": true,
			},
		),
	)
	if _, ok := result.(Supported); !ok {
		t.Errorf("Goos OR When IS NOT %[1]T (%[1]v)", result)
		return
	}
}

func TestAlwaysOrX(t *testing.T) {
	var result Constraint
	left := All{}

	// Always OR Never IS Always
	result = handleOrExpr(left, Ignored{})
	if _, ok := result.(All); !ok {
		t.Errorf("Always OR Never IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always OR Always IS Always
	result = handleOrExpr(left, All{})
	if _, ok := result.(All); !ok {
		t.Errorf("Always OR Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always OR Goos IS Goos
	result = handleOrExpr(left, Supported{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("Always OR Goos IS NOT %[1]v (%[1]v)", result)
		return
	}

	// Always OR When IS Always
	result = handleOrExpr(
		left,
		Platforms(
			map[string]bool{
				"linux":  true,
				"darwin": true,
			},
		),
	)
	if _, ok := result.(All); !ok {
		t.Errorf("Always OR When IS NOT %[1]v (%[1]v)", result)
		return
	}
}

func TestWhenOrX(t *testing.T) {
	var result Constraint
	left := Platforms(
		map[string]bool{
			"linux":  true,
			"darwin": true,
		},
	)

	// When OR Never IS When
	result = handleOrExpr(left, Ignored{})
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] && rWhen["darwin"] && len(rWhen) == 2) {
			t.Errorf("When OR Never - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("When OR Never IS NOT %[1]T", result)
		return
	}

	// When OR Always IS Always
	result = handleOrExpr(left, All{})
	if _, ok := result.(All); !ok {
		t.Errorf("When OR Always IS NOT %[1]v (%[1]v)", result)
		return
	}

	// When OR Goos IS Goos
	result = handleOrExpr(left, Supported{})
	if _, ok := result.(Supported); !ok {
		t.Errorf("When OR Goos IS NOT %[1]v (%[1]v)", result)
		return
	}

	// When OR When IS When IF UNION PARTIAL
	result = handleOrExpr(
		left,
		Platforms(
			map[string]bool{
				"freebsd": true,
				"openbsd": true,
				"zos":     true,
			},
		),
	)
	if rWhen, ok := result.(Platforms); ok {
		if !(rWhen["linux"] &&
			rWhen["zos"] &&
			rWhen["freebsd"] &&
			rWhen["openbsd"] &&
			rWhen["darwin"] &&
			len(rWhen) == 5) {
			t.Errorf("When OR When (PARTIAL) - Correct type but wrong tags %v", rWhen)
			return
		}
	} else {
		t.Errorf("When OR When (PARTIAL) IS NOT %T", result)
		return
	}

	// When OR When IS Always IF UNION FULL
	result = handleOrExpr(
		left,
		Platforms(
			map[string]bool{
				"aix":       true,
				"android":   true,
				"dragonfly": true,
				"hurd":      true,
				"illumos":   true,
				"ios":       true,
				"netbsd":    true,
				"solaris":   true,
			},
		),
	)
	if _, ok := result.(All); !ok {
		t.Errorf("When OR When (FULL) IS NOT %[1]T (%[1]v)", result)
		return
	}

}

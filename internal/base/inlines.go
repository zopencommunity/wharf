// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package base

import (
	_ "embed"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed inlines.yaml
var _DEFAULT_INLINES_EMBED []byte

var Inlines map[string]*PackageInline

const (
	// Explicit file handler types
	InlineDiffSym = "DIFF"

	// Explicit exported symbol handler types
	InlineExportSym = "EXPORT"
	InlineConstSym  = "CONST"
)

// Directive description for editting a specific file
//
// These directives are used either immediately or if automated porting fails
// as a means for porting packages using cached (user generated) changes
type FileInline struct {
	Type string
	Path string
}

// Directive description for editting definitions that cannot be ported
//
// If a package uses the specified definition we replace it with the contents provided here
type ExportInline struct {
	Type    string
	Replace string
}

// Directives related to a given package
type PackageInline struct {
	Files   map[string]FileInline
	Exports map[string]ExportInline
}

// Load the defaults on package init
func initInlines() {
	if err := yaml.Unmarshal(_DEFAULT_INLINES_EMBED, &Inlines); err != nil {
		panic("default explicits configuration file is formatted incorrectly")
	}
}

// Parse a given spec from source
func LoadInlines(file string) error {
	spec := make(map[string]*PackageInline)
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &Inlines)
	if err != nil {
		return err
	}

	for pkgname, pkgSpec := range spec {
		if defPkgSpec := Inlines[pkgname]; defPkgSpec != nil {
			for file, fileSpec := range pkgSpec.Files {
				defPkgSpec.Files[file] = fileSpec
			}
			for export, expSpec := range pkgSpec.Exports {
				defPkgSpec.Exports[export] = expSpec
			}
		} else {
			Inlines[pkgname] = pkgSpec
		}
	}

	return nil
}

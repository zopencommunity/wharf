// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package direct

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var _DEFAULTS_EMBED []byte

var Config map[string]*PackageDirective

const (
	// Explicit file handler types
	DiffType = "DIFF"

	// Explicit exported symbol handler types
	ExportType = "EXPORT"
	ConstType  = "CONST"
)

// Directive description for editting a specific file
//
// These directives are used either immediately or if automated porting fails
// as a means for porting packages using cached (user generated) changes
type FileDirective struct {
	Type string
	Path string
}

// Directive description for editting definitions that cannot be ported
//
// If a package uses the specified definition we replace it with the contents provided here
type ExportDirective struct {
	Type    string
	Replace string
}

// Directives related to a given package
type PackageDirective struct {
	Files   map[string]FileDirective
	Exports map[string]ExportDirective
}

// Load the defaults on package init
func init() {
	if err := yaml.Unmarshal(_DEFAULTS_EMBED, &Config); err != nil {
		panic("default explicits configuration file is formatted incorrectly")
	}
}

// Parse a given config from source
func ParseConfig(src []byte) (map[string]*PackageDirective, error) {
	var config map[string]*PackageDirective
	return config, yaml.Unmarshal(src, &config)
}

// Applies the given config on top of the current configuration
func Apply(overlay map[string]*PackageDirective) {
	for pkgname, opd := range overlay {
		if pd := Config[pkgname]; pd != nil {
			for file, ofd := range opd.Files {
				opd.Files[file] = ofd
			}
			for export, oed := range opd.Exports {
				opd.Exports[export] = oed
			}
		} else {
			Config[pkgname] = opd
		}
	}
}

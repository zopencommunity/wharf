// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package base

type Bundle struct {
	Modules  []Module
	Packages []Package

	Errors string `json:",omitempty"`

	GoWorkBackup string
	ImportDir    string
}

type Module struct {
	Path     string
	Version  string
	Fixed    string
	Dir      string `json:",omitempty"`
	Imported bool
}

type Package struct {
	Path       string
	Module     string
	Dir        string `json:",omitempty"`
	Actions    []map[string]any
	Files      []File
	TypeErrors []string
	Error      string `json:",omitempty"`
}

type File struct {
	Name     string
	Build    bool
	BaseFile string `json:",omitempty"`
	Lines    []Line `json:",omitempty"`
}

type Line struct {
	Line     uint
	Original string
	Fixed    string
	Changes  []LineChange
}

type LineChange struct {
	Column      uint
	Original    string
	Replacement string
}

type Error struct {
	IsPortError bool
	Error       string
}

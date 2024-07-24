// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package base

import "go/ast"

type Output struct {
	Modules  []ModulePin
	Packages []PackagePatch

	Errors string `json:",omitempty"`

	GoWorkBackup string `json:",omitempty"`
	ImportDir    string `json:",omitempty"`
}

type ModulePin struct {
	Path     string
	Version  string
	Pinned   string
	Imported bool   `json:",omitempty"`
	Dir      string `json:",omitempty"`
}

type PackagePatch struct {
	Path       string
	Dir        string
	Module     string
	Template   bool        `json:",omitempty"`
	Tags       []string    `json:",omitempty"`
	Files      []FilePatch `json:",omitempty"`
	TypeErrors []string
	Error      string `json:",omitempty"`
}

type FilePatch struct {
	Name     string
	Cached   string    `json:"-"`
	Syntax   *ast.File `json:"-"`
	Build    bool
	BaseFile string       `json:",omitempty"`
	Symbols  []SymbolRepl `json:",omitempty"`
	Lines    []LineDiff   `json:",omitempty"`
}

type SymbolRepl struct {
	Original string
	New      string
}

type LineDiff struct {
	Line     uint
	Original string
	New      string
}

type Error struct {
	IsPortError bool
	Error       string
}

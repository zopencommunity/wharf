// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package port2

type ActionList struct {
	Modules  []ModuleAction
	Packages []PackageAction
}

type ModuleAction struct {
	Path     string
	Version  string
	Fixed    string
	Dir      string `json:",omitempty"`
	Imported bool
}

type PackageAction struct {
	Path   string
	Module string
	Dir    string        `json:",omitempty"`
	Tags   []string      `json:",omitempty"`
	Tokens []TokenAction `json:",omitempty"`
	Files  []FileAction  `json:",omitempty"`
	Error  string        `json:",omitempty"`
}

type FileAction struct {
	Name     string
	Build    bool
	BaseFile string       `json:",omitempty"`
	Lines    []LineAction `json:",omitempty"`
}

type TokenAction struct {
	File   string
	Token  string
	Change string
}

type LineAction struct {
	Line     uint
	Original string
	Fixed    string
	Changes  []ChangeAction
}

type ChangeAction struct {
	Column      uint
	Original    string
	Replacement string
}

type Error struct {
	IsPortError bool
	Error       string
}

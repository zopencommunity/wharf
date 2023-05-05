// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package packages

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"regexp"
)

// Find the local import name for an import path
func FindImportName(file *ast.File, path string) *string {
	for _, ipt := range file.Imports {
		// String literals are wrapped in quotes, so we need to remove them
		imPath := ipt.Path.Value[1 : len(ipt.Path.Value)-1]

		if imPath == path {
			var name string
			if ipt.Name != nil {
				name = ipt.Name.Name
			} else {
				name = filepath.Base(path)
			}
			return &name
		}
	}

	return nil
}

// Errors came in the following formats:
// [symbol] not declared by package [pkg]
// undeclared name: [symbol]
// [v].[symbol] undefined (type [type] has no field or method [symbol])
// [v].[symbol] undefined (type [type] has no field or method [symbol], but does have [other])
var (
	// Go 1.20 matchers and older
	//

	_UNDEFINED_NAME_ERR_MATCHER_NEW = regexp.MustCompile(`undefined: (\w+)`)
	// undeclared: terminalWidth

	_UNDEFINED_PACKAGE_ERR_MATCHER_NEW = regexp.MustCompile(`undefined: (\w+)\.(\w+)`)
	// undeclared: syscall.EBADF

	// Pre Go 1.20 matchers
	//

	_UNDECLARED_NAME_ERR_MATCHER = regexp.MustCompile(`undeclared name: (\w+)`)
	// undeclared name: terminalWidth

	_UNDEFINED_TYPE_MEMBER_ERR_MATCHER = regexp.MustCompile(`(\w+\.\w+(?:\.\w+)?) undefined \(type \*?(?:(\w+)\.)?(\w+) has no field or method (\w+)`)
	// file.Close undefined (type File has no field or method

	_NOT_DECLARED_BY_PACKAGE_ERR_MATCHER = regexp.MustCompile(`(\w+) not declared by package (\w+)`)
	// EBADF not declared by package syscall
)

type TypeErrId interface {
	// Dummy function to limit types that can implement this interface
	teid()
}

type TypeError struct {
	Err    types.Error
	Reason TypeErrId
}

func (err TypeError) Error() string {
	return err.Err.Error()
}

type TCBadName struct {
	MemberOf *string
	Name     string
}

func (TCBadName) teid() {}

type TCBadImportName struct {
	Name    TCBadName
	PkgName string
}

func (TCBadImportName) teid() {}

type TCBadOther struct{}

func (TCBadOther) teid() {}

func parseTypeErrorReason(err types.Error) TypeErrId {
	if match := _UNDEFINED_PACKAGE_ERR_MATCHER_NEW.FindStringSubmatch(err.Msg); match != nil {
		return TCBadImportName{
			Name: TCBadName{
				Name: match[2],
			},
			PkgName: match[1],
		}
	} else if match := _UNDEFINED_NAME_ERR_MATCHER_NEW.FindStringSubmatch(err.Msg); match != nil {
		return TCBadName{
			Name: match[1],
		}
	} else if match := _UNDECLARED_NAME_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		return TCBadName{
			Name: match[1],
		}
	} else if match := _UNDEFINED_TYPE_MEMBER_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		une := TCBadName{
			MemberOf: &match[3],
			Name:     match[4],
		}
		if len(match[2]) > 0 {
			return TCBadImportName{
				Name:    une,
				PkgName: match[2],
			}
		}
		return une
	} else if match := _NOT_DECLARED_BY_PACKAGE_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		return TCBadImportName{
			Name: TCBadName{
				Name: match[1],
			},
			PkgName: match[2],
		}
	}

	return TCBadOther{}
}

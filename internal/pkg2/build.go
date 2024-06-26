// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package pkg2

import (
	"go/types"
	"regexp"
)

type Importer func(path string) (*types.Package, error)

func (f Importer) Import(path string) (*types.Package, error) {
	return f(path)
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

func NewTypeCheckError(err types.Error) (err2 TypeError) {
	err2.Err = err
	if match := _UNDEFINED_PACKAGE_ERR_MATCHER_NEW.FindStringSubmatch(err.Msg); match != nil {
		err2.Reason = TCBadImportName{
			Name: TCBadName{
				Name: match[2],
			},
			PkgName: match[1],
		}
	} else if match := _UNDEFINED_NAME_ERR_MATCHER_NEW.FindStringSubmatch(err.Msg); match != nil {
		err2.Reason = TCBadName{
			Name: match[1],
		}
	} else if match := _UNDECLARED_NAME_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		err2.Reason = TCBadName{
			Name: match[1],
		}
	} else if match := _UNDEFINED_TYPE_MEMBER_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		une := TCBadName{
			MemberOf: &match[3],
			Name:     match[4],
		}
		if len(match[2]) > 0 {
			err2.Reason = TCBadImportName{
				Name:    une,
				PkgName: match[2],
			}
		} else {
			err2.Reason = une
		}
	} else if match := _NOT_DECLARED_BY_PACKAGE_ERR_MATCHER.FindStringSubmatch(err.Msg); match != nil {
		err2.Reason = TCBadImportName{
			Name: TCBadName{
				Name: match[1],
			},
			PkgName: match[2],
		}
	} else {
		err2.Reason = TCBadOther{}
	}

	return
}

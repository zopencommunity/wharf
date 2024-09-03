// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
package util

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

func Format(src *ast.File, fset *token.FileSet) ([]byte, error) {
	b := new(strings.Builder)
	err := format.Node(b, fset, src)
	if err != nil {
		return nil, err
	}

	return []byte(b.String()), nil
}

// Adds the 'zos' build tag to a file
func AppendTagString(src []byte, tag string, op string, notice string) ([]byte, error) {
	var err error

	// Check for build tag, or add a new one
	buildIdx := bytes.Index(src, []byte("//go:build"))
	if buildIdx > -1 {
		if len(op) > 0 {
			replacedText := bytes.Replace(
				src[buildIdx:],
				[]byte("\n"),
				[]byte(fmt.Sprintf(" %v %v\n// %v\n", op, tag, notice)),
				1,
			)
			src = append(src[0:buildIdx], replacedText...)
		} else {
			eol := buildIdx + bytes.Index(src[buildIdx:], []byte("\n"))
			src = append(
				append(
					src[0:buildIdx],
					[]byte(fmt.Sprintf("//go:build %v\n//%v\n", tag, notice))...,
				),
				src[eol+1:]...,
			)
		}
	} else {
		src = append([]byte(fmt.Sprintf("//go:build %v\n//%v\n", tag, notice)), src...)
	}

	// Need to reformat in case //+build tags exist
	src, err = format.Source(src)
	if err != nil {
		return nil, err
	}

	return src, nil
}

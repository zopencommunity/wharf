// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package tags

import (
	"bytes"
	"errors"
	"fmt"
	"go/build/constraint"
	"strings"
	"regexp"
	"os"
	"path/filepath"
)

// All the unix-like platforms, listed in order of build priority
// Must match 'unixOS' list below
var UNIX_PLATFORM_RANKING = []string{
	"linux",
	"openbsd",
	"freebsd",
	"netbsd",
	"darwin",
	"solaris",
	"illumos",
	"dragonfly",
	"android",
	"ios",
	"hurd",
	"aix",
}

var knownOS = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"hurd":      true,
	"illumos":   true,
	"ios":       true,
	"js":        true,
	"linux":     true,
	"nacl":      true,
	"netbsd":    true,
	"openbsd":   true,
	"plan9":     true,
	"solaris":   true,
	"windows":   true,
	"zos":       true,
}

// unixOS is the set of GOOS values matched by the "unix" build tag.
//
// The contents of this are based on $GOROOT/src/go/build/syslist.go
var unixOS = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"hurd":      true,
	"illumos":   true,
	"ios":       true,
	"linux":     true,
	"netbsd":    true,
	"openbsd":   true,
	"solaris":   true,
	"zos":       true,
}

var knownArch = map[string]bool{
	"386":         true,
	"amd64":       true,
	"amd64p32":    true,
	"arm":         true,
	"armbe":       true,
	"arm64":       true,
	"arm64be":     true,
	"loong64":     true,
	"mips":        true,
	"mipsle":      true,
	"mips64":      true,
	"mips64le":    true,
	"mips64p32":   true,
	"mips64p32le": true,
	"ppc":         true,
	"ppc64":       true,
	"ppc64le":     true,
	"riscv":       true,
	"riscv64":     true,
	"s390":        true,
	"s390x":       true,
	"sparc":       true,
	"sparc64":     true,
	"wasm":        true,
}

type Constraint interface{}

type Ignored struct{}

type Platforms map[string]bool

type Supported struct{}

type All struct{}

func Parse(name string, src []byte, goos string, buildtags map[string]bool) Constraint {
	nametag, ok := ParseFileName(name)
	if !ok {
		return Ignored{}
	}

	// An invalid line is a "soft" error
	//
	// Don't report it, but mark the file as to not build
	// (similarly to how the go compiler would handle it)
	expr, err2 := ParseFileHeader(src)
	if err2 != nil {
		return Ignored{}
	} else if expr != nil {
		// Add in nametag if exists
		if nametag != nil {
			expr = &constraint.AndExpr{
				X: nametag,
				Y: expr,
			}
		}

		return parseTagExpr(
			expr,
			false,
			&goos,
			buildtags,
		)

	} else if nametag != nil {
		return Platforms(
			map[string]bool{
				nametag.Tag: true,
			},
		)
	} else {
		return All{}
	}
}

func ParseFileName(name string) (nametag *constraint.TagExpr, ok bool) {
	name = strings.TrimSuffix(name, ".go")

	// Files named name_test.go can show up as IgnoredFiles and not Test files
	if strings.HasSuffix(name, "_test") {
		// TODO: this is a temp work around
		return nil, false
	}

	idx := strings.LastIndexByte(name, byte('_'))
	// Check for GOARCH tag in name
	if idx > 0 && idx < len(name)-1 {
		tag := name[idx+1:]
		if knownArch[tag] {
			// TODO: This should not be hardcoded in
			if tag != "s390x" {
				// Files that are for a different GOARCH are a DO NOT USE
				return
			}
			// Trim the tag from the filename (setup to handle GOOS in next step)
			name = name[:idx]
			idx = strings.LastIndexByte(name, byte('_'))
		}
	}

	// An early return indicates that we failed the GOARCH test
	ok = true

	if idx > 0 && idx < len(name)-1 {
		tag := name[idx+1:]
		if knownOS[tag] {
			if unixOS[tag] {
				nametag = &constraint.TagExpr{
					Tag: tag,
				}
			} else {
				ok = false
			}
		}
	}
	return
}

// This algorithm determines what platforms a file will build under
//
// Specifically it will report back the unix-like platforms (GOOS values)
// that would cause this file to be built. The list will only include goos
// IF AND ONLY IF the tag for the current GOOS is SPECIFICALLY added. A file may build on the current GOOS
// (for example) a file tagged for 'unix' or a file with no tag constraints,
// for situations like those it is unclear if the developer developed with the GOOS in mind
// however for situations where a GOOS tag exists, it is obvious that this file is meant for GOOS
//
// To build this list we have the following rules:
// 1. Treat the tag GOARCH as 'unix'
// 2. Expand the tag 'unix' to the full set of unixOS tags
// 2. Only report back tags that fall under the 'unix' definition (as defined in syslist.go)
// 3. Treat all other tags (including non-unix GOOS tags, GOARCH tags, and build tags) as FALSE

// Set representation of platform constraints
//
// Constraints are representative of inclusive OR statements for TRUE members
// Ex: linux || darwin
//
// Since GOOS values are mutually exclusive we can treat missing/off values
// as a combination of AND statements attached to the OR when performing
// AND operations with other constraints, and can be ignored when performing OR operations
// Ex: (linux || darwin) && !aix && !freebsd && ...

// Parses the tag expression into a constraint
func parseTagExpr(expr constraint.Expr, negate bool, goos *string, atags map[string]bool) Constraint {
	switch expr2 := expr.(type) {
	case *constraint.OrExpr:
		// x || y

		if negate {
			// !(x || y) == !x && !y
			return handleAndExpr(
				parseTagExpr(expr2.X, negate, goos, atags),
				parseTagExpr(expr2.Y, negate, goos, atags),
				*goos,
			)
		}

		return handleOrExpr(
			parseTagExpr(expr2.X, negate, goos, atags),
			parseTagExpr(expr2.Y, negate, goos, atags),
		)

	case *constraint.AndExpr:
		// x && y

		if negate {
			// !(x && y) == !x || !y
			return handleOrExpr(
				parseTagExpr(expr2.X, negate, goos, atags),
				parseTagExpr(expr2.Y, negate, goos, atags),
			)
		}

		return handleAndExpr(
			parseTagExpr(expr2.X, negate, goos, atags),
			parseTagExpr(expr2.Y, negate, goos, atags),
			*goos,
		)

	case *constraint.NotExpr:
		// !x
		return parseTagExpr(expr2.X, !negate, goos, atags)

	case *constraint.TagExpr:
		// x
		return handleTagExpr(expr2.Tag, negate, *goos, atags)

	default:
		panic(fmt.Errorf("golang: unknown tag expression type: %T", expr2))
	}
}

func handleTagExpr(tag string, negate bool, goos string, atags map[string]bool) Constraint {
	// Return a set for any unix OS tags
	if unixOS[tag] {
		// The GOOS flag either signifies a "never" or a "GOOS" specifically (treat it as a never if negated)
		if tag == goos {
			if negate {
				return Ignored{}
			}
			return Supported{}
		}

		// !tag => build on all except 'tag'
		if negate {
			cstr := Platforms(
				make(map[string]bool, len(unixOS)),
			)
			for os := range unixOS {
				cstr[os] = true
			}
			delete(cstr, tag)
			return cstr
		}

		cstr := Platforms(
			make(map[string]bool, 1),
		)
		cstr[tag] = true
		return cstr
	}

	// Fallback logic is reversed for 'unix' and provided build tags
	if tag == "unix" || atags[tag] {
		negate = !negate
	}

	// Fallback logic is to treat it as some unset build tag
	// Therefore !x == TRUE and x == FALSE
	if negate {
		return All{}
	}
	return Ignored{}
}

func handleOrExpr(left, right Constraint) Constraint {
	// Cases (in priority)
	// - GOOS || ANYTHING => GOOS
	// - ALWAYS || ANYTHING => ALWAYS
	// - NEVER || ANYTHING => ANYTHING
	// - TAGS || ANYTHING => see below

	if _, ok := right.(Supported); ok {
		return Supported{}
	}

	switch l2 := left.(type) {
	case Supported:
		return Supported{}
	case All:
		return All{}
	case Ignored:
		return right
	case Platforms:
		// GOOS case handled above for both sides
		// Possible cases:
		// - TAGS || TAGS => TAGS U TAGS
		// - TAGS || NEVER => TAGS
		// - TAGS || ALWAYS => ALWAYS
		if r2, ok := right.(Platforms); ok {
			// Merge by a union
			for tag, ok := range r2 {
				if ok {
					l2[tag] = true
				}
			}
			if len(l2) == len(unixOS) {
				return All{}
			}
			return l2
		} else if _, ok := right.(Ignored); ok {
			return l2
		} else if _, ok := right.(All); ok {
			return All{}
		} else {
			panic("impossible build constraint for OR")
		}
	}

	panic("all OR cases should have been handled above")
}

func handleAndExpr(left, right Constraint, goos string) Constraint {

	// Cases (in priority)
	// NEVER && ANYTHING => NEVER
	// ALWAYS && ANYTHING => ANYTHING
	// GOOS && ANYTHING => see below
	// TAG && ANYTHING => see below
	if _, ok := right.(Ignored); ok {
		return Ignored{}
	}

	// Quickly check if the right side covers GOOS
	rgoos := true
	if r2, ok := right.(Platforms); ok {
		rgoos = r2[goos]
	}

	switch l2 := left.(type) {
	case Ignored:
		return Ignored{}
	case All:
		return right
	case Supported:
		// If right side does not cover GOOS then NEVER otherwise GOOS
		// (... || goos) && (...) => we have GOOS covered in the build tags
		// therefore we assume this file was editted to handle GOOS case
		if rgoos {
			return Supported{}
		}
		return Ignored{}
	case Platforms:
		// NEVER case handled above for both sides
		// Possible cases:
		// - TAGS && TAGS => TAGS ∩ TAGS
		// - TAGS && GOOS => GOOS <=> TAGS contains GOOS otherwise NEVER
		// - TAGS && ALWAYS => TAGS
		if r2, ok := right.(Platforms); ok {
			// Merge by intersection
			result := Platforms(
				// TODO: set this to an appropriate size other than just assuming r2 is smaller
				make(map[string]bool, len(r2)),
			)

			for os := range unixOS {
				if r2[os] && l2[os] {
					result[os] = true
				}
			}

			if len(result) == 0 {
				return Ignored{}
			}

			return result
		} else if _, ok := right.(Supported); ok {
			// See above for reason we do this
			if l2[goos] {
				return Supported{}
			}
			return Ignored{}
		} else if _, ok := right.(All); ok {
			return l2
		} else {
			panic("impossible build constraint for OR")
		}
	}

	panic("all AND cases should have been handled above")
}

var (
	slashSlash = []byte("//")
	slashStar  = []byte("/*")
	starSlash  = []byte("*/")

	bSlashSlash = []byte(slashSlash)
	bSlashStar  = []byte(slashStar)
	bPlusBuild  = []byte("+build")

	goBuildComment = []byte("//go:build")

	errMultipleGoBuild = errors.New("multiple //go:build comments")
)

func isGoBuildComment(line []byte) bool {
	if !bytes.HasPrefix(line, goBuildComment) {
		return false
	}
	line = bytes.TrimSpace(line)
	rest := line[len(goBuildComment):]
	return len(rest) == 0 || len(bytes.TrimSpace(rest)) < len(rest)
}

func findPlusBuild(content []byte) constraint.Expr {
	var x constraint.Expr
	p := content
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, bSlashSlash) || !bytes.Contains(line, bPlusBuild) {
			continue
		}
		text := string(line)
		if !constraint.IsPlusBuild(text) {
			continue
		}
		if y, err := constraint.Parse(text); err == nil {
			// Separate +build lines act as AND cases
			if x != nil {
				x = &constraint.AndExpr{
					X: x,
					Y: y,
				}
			} else {
				x = y
			}
		}
	}
	return x
}

func ParseFileHeader(content []byte) (constraint.Expr, error) {
	tagLine, err := findGoBuild(content)
	if err != nil {
		return nil, err
	}

	if tagLine != nil {
		return constraint.Parse(string(tagLine))
	}

	// No go:build line, therefore by Go spec any +build lines are used
	return findPlusBuild(content), nil
}

func findGoBuild(content []byte) (goBuild []byte, err error) {
	p := content
	ended := false       // found non-blank, non-// line, so stopped accepting // +build lines
	inSlashStar := false // in /* */ comment

Lines:
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 && !ended { // Blank line
			// Remember position of most recent blank line.
			// When we find the first non-blank, non-// line,
			// this "end" position marks the latest file position
			// where a // +build line can appear.
			// (It must appear _before_ a blank line before the non-blank, non-// line.
			// Yes, that's confusing, which is part of why we moved to //go:build lines.)
			// Note that ended==false here means that inSlashStar==false,
			// since seeing a /* would have set ended==true.
			continue Lines
		}
		if !bytes.HasPrefix(line, slashSlash) { // Not comment line
			ended = true
		}

		if !inSlashStar && isGoBuildComment(line) {
			if goBuild != nil {
				return nil, errMultipleGoBuild
			}
			goBuild = line
		}

	Comments:
		for len(line) > 0 {
			if inSlashStar {
				if i := bytes.Index(line, starSlash); i >= 0 {
					inSlashStar = false
					line = bytes.TrimSpace(line[i+len(starSlash):])
					continue Comments
				}
				continue Lines
			}
			if bytes.HasPrefix(line, bSlashSlash) {
				continue Lines
			}
			if bytes.HasPrefix(line, bSlashStar) {
				inSlashStar = true
				line = bytes.TrimSpace(line[len(bSlashStar):])
				continue Comments
			}
			// Found non-comment text.
			break Lines
		}
	}

	return goBuild, nil
}

func FindPackageName(baseDir string, goFiles []string) string {
	packageExp := regexp.MustCompile("\n*\\s*package\\s+([a-zA-Z_]+)\\s*")
	testFileExp := regexp.MustCompile("^[a-zA-Z0-0_]_test.go$")
	for _, file := range goFiles {
		if testFileExp.MatchString(file) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(baseDir, file))
		if err != nil {
			continue
		}
		if res := packageExp.FindSubmatch(content); len(res) > 0 {
			return string(res[1])
		}
	}
	return ""
}
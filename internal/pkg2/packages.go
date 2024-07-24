// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package pkg2

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zosopentools/wharf/internal/tags"
)

const CGO_PACKAGE_NAME = "C"
const UNSAFE_PACKAGE_NAME = "unsafe"
const GOLANGX_PATH_PREFIX = "golang.org/x/"

var FileSet = token.NewFileSet()

var backupLookupMap = make(map[string]*Package)

// Go list error for when no files in a package are built
var _BUILD_CONSTRAINTS_EXCLUDE_ALL_FILE = regexp.MustCompile(`build constraints exclude all Go files in ([a-zA-Z0-9_/@.]+)`)

// Package is a golang.org/x/... package
func IsGolangXPkg(pkg *Package) bool {
	return pkg.Meta.Module != nil && strings.HasPrefix(pkg.Meta.Module.Path, "golang.org/x/")
}

func IsStdlibPkg(pkg *Package) bool {
	return pkg.Meta.Goroot || pkg.Meta.Standard || IsGolangXPkg(pkg)
}

func IsExcludeGoListError(errMessage string) bool {
	return _BUILD_CONSTRAINTS_EXCLUDE_ALL_FILE.MatchString(errMessage)
}

func BackupNameLookup(name string) *Package {
	return backupLookupMap[name]
}

func ImportPathToAssumedName(importPath string) (string, string) {
	alt := ""
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			dir := path.Dir(importPath)
			if dir != "." {
				alt = base
				base = path.Base(dir)
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, notIdentifier); i >= 0 {
		base = base[:i]
	}
	return base, alt
}

func notIdentifier(ch rune) bool {
	return !('a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' ||
		'0' <= ch && ch <= '9' ||
		ch == '_' ||
		ch >= utf8.RuneSelf && (unicode.IsLetter(ch) || unicode.IsDigit(ch)))
}

type ImportTree struct {
	resolved bool
	from     []*Package
	groups   [][]*Package
}

func (tree *ImportTree) Groups() [][]*Package {
	if !tree.resolved {
		panic("attempted acquire of groups on unresolved import tree (call ImportTree.Resolve before ImportTree.Groups)")
	}
	return tree.groups
}

// Perform recursive DFS on the tree
//
// During the search:
// - Build topology
// - Check for cycles
// - Build exported types for packages
func (tree *ImportTree) Resolve() error {
	layers := make([][]*Package, 0, 30)
	layers = append(layers, make([]*Package, 0))
	visited := make(map[string]bool, len(cache))

	var visit func(pkg *Package) (int, error)
	visit = func(pkg *Package) (int, error) {
		// Handle cases where we have visited the node already
		if done, checked := visited[pkg.Meta.ImportPath]; done {
			return pkg.level, nil
		} else if checked {
			return -1, &importCycleError{
				stack: []string{
					pkg.Meta.ImportPath,
				},
			}
		}

		// Mark so that if we see it again, we know we have a cycle
		visited[pkg.Meta.ImportPath] = false

		level := 0

		// Visit each import path
		for _, ipkg := range pkg.Imports {
			ipkg.Parents = append(ipkg.Parents, pkg)

			seenlevel, err := visit(ipkg)
			if err != nil {
				// Create traceback for import cycles
				if ice, ok := err.(*importCycleError); ok {
					ice.stack = append(ice.stack, pkg.Meta.ImportPath)
				}
				return -1, err
			}

			pkg.DepDirty = pkg.DepDirty || ipkg.DepDirty || ipkg.Dirty || ipkg.Modified
			// If the child's level is higher or identical to our currently known level we move up
			if seenlevel >= level {
				level = seenlevel + 1
			}
		}

		// We now have a known level, so we attach the import layer
		for len(layers) <= level {
			layers = append(layers, make([]*Package, 0))
		}

		layers[level] = append(layers[level], pkg)
		pkg.level = level
		visited[pkg.Meta.ImportPath] = true

		return level, nil
	}

	for _, pkg := range tree.from {
		if done, checked := visited[pkg.Meta.ImportPath]; done {
			// Node already visited, pass
			continue
		} else if checked {
			panic("package is marked visited when no DFS is currently being performed")
		} else {
			// DFS on this node
			_, err := visit(pkg)
			if err != nil {
				return err
			}
		}
	}

	tree.resolved = true
	tree.groups = layers
	return nil
}

type Package struct {
	// Metadata
	Meta *MetaPackage

	// Builds
	Builds []BuildConfig

	// Files
	Files map[string]*GoFile

	// Packages it is imported by
	Parents []*Package
	// Imported packages
	Imports map[string]*Package

	// Whether or not the package would actually get built in the default environment
	Included bool

	// Set if the packages source has changed
	Dirty bool

	// Set if a dependency (direct or in-direct) is dirty
	DepDirty bool

	// Internal control mechanism for setting modified status
	modified bool

	// Set if the package has been modified manually
	Modified bool

	// Set if this is the first time the package is loaded
	FirstLoad bool

	// Level of the tree the package is on
	level int

	// Any errors that occurred during load
	Errors []error
}

func (pkg *Package) LoadSyntax(build int) error {
	cfg := &pkg.Builds[build]
	// Make sure we have the syntax loaded
	if cfg.Syntax == nil {
		for _, gofile := range cfg.Files {
			if gofile.Syntax == nil {
				src, err := os.ReadFile(gofile.Path)
				if err != nil {
					return err
				}

				parsed, err := parser.ParseFile(FileSet, gofile.Name, src, 0)
				if err != nil {
					return err
				}
				gofile.Syntax = parsed
			}
			cfg.Syntax = append(cfg.Syntax, gofile.Syntax)
		}
	}
	return nil
}

func (pkg *Package) LookupImport(pkgName string, fileName string) *Package {
	file := pkg.Files[fileName]
	if file.Imports[pkgName] != "" {
		return pkg.Imports[file.Imports[pkgName]]
	} else if backup := BackupNameLookup(pkgName); backup != nil {
		return backup
	} else {
		return nil
	}
}

func (pkg *Package) MarkModified() {
	pkg.modified = true
}

func (pkg *Package) String() string {
	if pkg == nil {
		return "<nil>"
	}
	return pkg.Meta.ImportPath
}

func (ice *importCycleError) Error() string {
	var sb strings.Builder
	sb.WriteString("import cycle detected:\n")
	sb.WriteString(ice.stack[0])
	for _, pkg := range ice.stack[1:] {
		sb.WriteString("\n<- ")
		sb.WriteString(pkg)
		if pkg == ice.stack[0] {
			sb.WriteString(" --- SEEN HERE BEFORE")
		}
	}

	return sb.String()
}

type importCycleError struct {
	stack []string
}

type BuildConfig struct {
	Platforms []string
	Files     []*GoFile
	Syntax    []*ast.File
}

type GoFile struct {
	Name        string
	Path        string
	Cgo         bool
	Default     bool
	Syntax      *ast.File
	Tags        tags.Constraint
	Imports     map[string]string
	AnonImports []string
	Replaced    *ReplacedFile
}

func (gf *GoFile) String() string {
	if gf == nil {
		return "<nil>"
	}
	return gf.Name
}

type ReplacedFile struct {
	File   *GoFile
	Reason any
}

// Fields that are commented out are provided by go-list
// however that are not need by wharf so leave them
// commented out in case they ever do become useful
type MetaPackage struct {
	Dir        string // directory containing package sources
	ImportPath string // import path of package in dir
	// ImportComment string // path in import comment on package statement
	Name string // package name
	// Shlib         string // the shared library that contains this package (only set when -linkshared)
	Goroot   bool // is this package in the Go root?
	Standard bool // is this package part of the standard Go library?
	// Stale         bool   // would 'go install' do anything for this package?
	// StaleReason   string // explanation for Stale==true
	// Root string // Go root or Go path dir containing this package
	// ConflictDir   string // this directory shadows Dir in $GOPATH
	// BinaryOnly bool // binary-only package (no longer supported)
	// ForTest       string      // package is only for use in named test
	Export string // file containing export data (when using -export)
	// BuildID       string      // build ID of the compiled package (when using -export)
	Module  *Module  // info about package's containing module, if any (can be nil)
	Match   []string // command-line patterns matching this package
	DepOnly bool     // package is only a dependency, not explicitly

	// Source files
	GoFiles  []string // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
	CgoFiles []string // .go source files that import "C"
	// CompiledGoFiles   []string // .go files presented to compiler (when using -compiled)
	IgnoredGoFiles []string // .go source files ignored due to build constraints
	// IgnoredOtherFiles []string // non-.go source files ignored due to build constraints
	// CFiles            []string // .c source files
	// CXXFiles          []string // .cc, .cxx and .cpp source files
	// MFiles            []string // .m source files
	// HFiles            []string // .h, .hh, .hpp and .hxx source files
	// FFiles            []string // .f, .F, .for and .f90 Fortran source files
	// SFiles            []string // .s source files
	// SwigFiles         []string // .swig files
	// SwigCXXFiles      []string // .swigcxx files
	// SysoFiles         []string // .syso object files to add to archive
	// TestGoFiles  []string // _test.go files in package
	// XTestGoFiles []string // _test.go files outside package

	// Embedded files
	// EmbedPatterns []string // //go:embed patterns
	// EmbedFiles    []string // files matched by EmbedPatterns
	// TestEmbedPatterns  []string // //go:embed patterns in TestGoFiles
	// TestEmbedFiles     []string // files matched by TestEmbedPatterns
	// XTestEmbedPatterns []string // //go:embed patterns in XTestGoFiles
	// XTestEmbedFiles    []string // files matched by XTestEmbedPatterns

	// Cgo directives
	// CgoCFLAGS    []string // cgo: flags for C compiler
	// CgoCPPFLAGS  []string // cgo: flags for C preprocessor
	// CgoCXXFLAGS  []string // cgo: flags for C++ compiler
	// CgoFFLAGS    []string // cgo: flags for Fortran compiler
	// CgoLDFLAGS   []string // cgo: flags for linker
	// CgoPkgConfig []string // cgo: pkg-config names

	// Dependency information
	Imports   []string          // import paths used by this package
	ImportMap map[string]string // map from source import to ImportPath (identity entries omitted)
	// Deps         []string          // all (recursively) imported dependencies
	// TestImports  []string          // imports from TestGoFiles
	// XTestImports []string          // imports from XTestGoFiles

	// Error information
	Incomplete bool                // this package or a dependency has an error
	Error      *JsonPackageError   // error loading package
	DepsErrors []*JsonPackageError // errors loading dependencies
}

type JsonPackageError struct {
	ImportStack []string // shortest path from package named on command line to this one
	Pos         string   // position of error (if present, file:line:col)
	Err         string   // the error itself
}

type Module struct {
	Path string // module path
	// Query   string // version query corresponding to this version
	Version string // module version
	// Versions []string    // available module versions
	Replace *Module // replaced by this module
	// Time     *time.Time  // time version was created
	// Update     *Module      // available update (with -u)
	Main      bool   // is this the main module?
	Indirect  bool   // module is only indirectly needed by main module
	Dir       string // directory holding local copy of files, if any
	GoMod     string // path to go.mod file describing module, if any
	GoVersion string // go version used in module
	// Retracted  []string         // retraction information, if any (with -retracted or -u)
	// Deprecated string           // deprecation message, if any (with -u)
	Error *ModuleError // error loading module
}

type ModuleError struct {
	Err string // the error itself
}

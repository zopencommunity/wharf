// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package packages

import (
	"go/ast"
	"go/token"
	"go/types"
)

// These shouldn't change so we can have these as globals for easy reference
var Goos string
var BuildTags map[string]bool
var PackageImportGraph []*ProcGroup

var globalpkgs map[string]*Package = make(map[string]*Package, 50)
var globalName2IName map[string]string = make(map[string]string, 10)
var globalImportMap map[string]map[string]string = make(map[string]map[string]string, 10)

func GetGlobalPkg(importName string) *Package {
	return globalpkgs[importName]
}

func PkgName2ImportName(pkgName string) string {
	return globalName2IName[pkgName]
}

func ClearGlobalpkgs() {
	globalpkgs = make(map[string]*Package, 0)
}

func ClearGlobalName2IName() {
	globalName2IName = make(map[string]string, 0)
}

func ClearAll() {
        ClearGlobalpkgs()
        ClearGlobalName2IName()
        PackageImportGraph = PackageImportGraph[:]
}

func GetPathFromImportMap(pkgImportPath string, depsPath string) string {
       if importMap, ok := globalImportMap[pkgImportPath]; ok {
               return importMap[depsPath]
       }
       return ""
}

type ProcGroup struct {
	Packages []*Package

	Next *ProcGroup

	Level int
}

// Information regarding how the package was discovered
type RefType uint8

const (
	Inactive   RefType = iota // Package is unused in default configuration
	Dependency                // Package would be built as it is dependended upon
	Main                      // Package was specified in provided arguments
)

// Additional configuration options for load behaviour
type LoadOption uint

const (
	LoadHaltMistyped LoadOption = 1 << iota // Halt loading on mistyped (reserved for stdlib packages)
	LoadAllConfigs                          // Load all source files and parse configs
)

// State for validation during loading
type ldcolor uint8

const (
	// Stage 1 mask values
	unloaded ldcolor = iota
	firstload
	// Stage 2 mask values
	searchable
	searching
	searched
)

type Package struct {
	// Metadata
	Dir        string
	ImportPath string
	Name       string
	Goroot     bool
	Export     string
	Module     *Module

	// Layer the package is on
	Layer *ProcGroup

	// Used in / Imported packages
	Parents []*Package
	Imports map[string]*Package

	// Build Configurations
	//
	// Index 0 is the default build configuration
	// The rest are ordered using the most generic unix-like ordering (see tags.go)
	Configs []BuildConfig

	// Active config
	//
	// Config to use when attempting a build
	CfgIdx int

	// Map file names and their import names to paths
	FileImports map[string]map[string]string

	// Fileset for loading all files for this package
	Fset *token.FileSet

	// Whether or not the package would actually get built in the default environment
	Active bool

	// Flags used for external processing (the porting package uses this, has no impact on load)
	ExtFlags uint8

	// Mark used for performing DFS
	color ldcolor

	// Partially loaded type data
	Types *types.Package

	// Any errors that occurred during load
	Errors []error
}

func (pkg *Package) String() string {
	if pkg == nil {
		return "<nil>"
	}
	return pkg.ImportPath
}

type GoFile struct {
	Name string

	Syntax *ast.File

	IsCgo bool
}

type ExtGoFile struct {
	Path string

	Syntax *ast.File

	Meta any
}

func (gf *GoFile) String() string {
	if gf == nil {
		return "<nil>"
	}
	return gf.Name
}

type BuildConfig struct {
	Platforms []string
	GoFiles   []*GoFile
	Syntax    []*ast.File
	Override  map[string]*ExtGoFile
}

type Importer func(path string) (*types.Package, error)

type iImporter Importer

func (f iImporter) Import(path string) (*types.Package, error) {
	return f(path)
}

// Fields that are commented out are provided by go-list
// however that are not need by wharf so leave them
// commented out in case they ever do become useful
type RawPackage struct {
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
	Incomplete bool               // this package or a dependency has an error
	Error      *rawPackageError   // error loading package
	DepsErrors []*rawPackageError // errors loading dependencies
}

type rawPackageError struct {
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

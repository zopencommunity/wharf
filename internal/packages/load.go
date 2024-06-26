// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package packages

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	genpath "path"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/tags"
	"github.com/zosopentools/wharf/internal/util"
)

type importCycleError struct {
	stack []string
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

func Load(paths []string, getOpts func(RawPackage, bool) (LoadOption, error)) (*ProcGroup, error) {
	// Global package reference (used for determining packages later on)
	loadedPkgs := make(map[string]*Package, len(globalpkgs))
	loadedModules := make(map[string]*Module, 10)
	toLoadPkgs := make(map[string]bool, 10)

	// Cached layer stack
	layers := make([]*ProcGroup, 0, 30)
	layers = append(layers, &ProcGroup{
		Level: 0,
	})

	// Setup for loading from CLI args
	next := paths
	cli := true
	clipkgs := make([]*Package, 0, len(paths))

	loads := make([]int, 0, 10)

	// //// / //
	// PASS 1 //
	// //// / //

	// Use go-list to perform a BFS and build the initial tree
	//
	// Do first load (command line listed packages)
	for len(next) > 0 {
		loads = append(loads, len(next))
		// Load from go-list
		listout, err := util.GoList(next)
		if err != nil {
			return nil, err
		}

		var jpkgs []RawPackage
		decoder := json.NewDecoder(strings.NewReader(listout))
		for decoder.More() {
			// Possibility that the package list resolved to only a single package
			jpkgs = append(jpkgs, RawPackage{})
			if err := decoder.Decode(&jpkgs[len(jpkgs)-1]); err != nil {
				// TODO: provide better error
				panic(err)
			}
		}

		if len(jpkgs) == 0 {
			return nil, fmt.Errorf("no packages found using the given path")
		}

		next = make([]string, 0, len(jpkgs))

		register := func(path string) *Package {
			pkg := loadedPkgs[path]
			if pkg == nil {
				// First time we saw the package this run
				pkg = globalpkgs[path]
				if pkg == nil {
					// First time seeing the package ever
					pkg = new(Package)
					pkg.color = firstload
					globalpkgs[path] = pkg
				}

				loadedPkgs[path] = pkg

				next = append(next, path)
				toLoadPkgs[path] = true
			}
			return pkg
		}

		for _, jpkg := range jpkgs {
			// Lookup package or create new entry
			pkg := register(jpkg.ImportPath)
			delete(toLoadPkgs, jpkg.ImportPath)

			if pkg.color == searchable {
				continue
			}

			// Lazy check to determine the package hasn't changed (will reload information if there is module inconsitency)
			doLoad := pkg.color == firstload || pkg.Dir != jpkg.Dir

			// Use a common module object
			// On first load we see if we can find the module from the cache
			// If not, default to the module from go-list
			if jpkg.Module != nil {
				if pkg.color != firstload {
					// Simple sanity check to see if a module randomly popped into existence
					if pkg.Module == nil {
						// TODO: this should be an error and not a panic
						panic("missing module data for package that is now part of a module")
					}

					// Additional lazy check to make sure the module hasn't changed
					if !doLoad {
						// Check module data, could hold same contents just be different pointers (because of how serialization works)
						if jpkg.Module == nil {
							// TODO: this should be an error not a panic
							panic("missing module data for package that was part of a module")
						}

						cmod := pkg.Module
						if cmod.Replace != nil {
							cmod = cmod.Replace
						}

						nmod := jpkg.Module
						if nmod.Replace != nil {
							nmod = nmod.Replace
						}

						doLoad = cmod.Path != nmod.Path || cmod.Version != nmod.Version
					}
				}

				// Need to use the new module (even if they are the same)
				// because it is faster and safer than manually checking differences
				pkg.Module = loadedModules[jpkg.Module.Path]
				if pkg.Module == nil {
					loadedModules[jpkg.Module.Path] = jpkg.Module
					pkg.Module = jpkg.Module
				}

			}

			if cli {
				pkg.Active = true
				if !jpkg.DepOnly {
					clipkgs = append(clipkgs, pkg)
				}
			}

			// go-list errors mean the environment is bad -> stop loading for bad environments
			if jpkg.Error != nil {
				// if the package does not contain any go file can be build on the platform
				if _BUILD_CONSTRAINS_EXCLUDE_ALL_FILE.MatchString(jpkg.Error.Err) {
					// since the file will be ignore go list does not read any of the go file
					// It can not find a package name. Need to find the package manually
					if jpkg.Name == "" {
						jpkg.Name = tags.FindPackageName(jpkg.Dir, jpkg.IgnoredGoFiles)
					}
				} else {
					return nil, fmt.Errorf("unable to load %v: %v", jpkg.ImportPath, jpkg.Error.Err)
				}
			}

			opts, err := getOpts(jpkg, cli)
			if err != nil {
				return nil, fmt.Errorf("unable to load %v: %v", jpkg.ImportPath, err)
			}

			if doLoad {
				pkg.Dir = jpkg.Dir
				pkg.ImportPath = jpkg.ImportPath
				pkg.Name = jpkg.Name
				pkg.Goroot = jpkg.Goroot
				pkg.Export = jpkg.Export
				pkg.Imports = make(map[string]*Package, len(jpkg.Imports))
				pkg.Fset = token.NewFileSet()
				pkg.Types = nil

				if opts&LoadAllConfigs != 0 {
					pkg.FileImports = make(map[string]map[string]string, len(jpkg.GoFiles)+len(jpkg.CgoFiles)+len(jpkg.IgnoredGoFiles))
				} else {
					pkg.FileImports = nil
				}

				if len(jpkg.ImportMap) > 0 {
					globalImportMap[jpkg.ImportPath] = jpkg.ImportMap
				}

				impMask := make(map[string]bool, len(jpkg.Imports))
				found := 0 // For sanity check

				// TODO: don't use load build configs if we aren't loading all the configs
				pkg.Configs, err = loadBuildConfigs(
					pkg.Fset,
					&jpkg,
					opts&LoadAllConfigs != 0,
					func(file, name, ipath string, built bool, count int) {
						// ImportMap contains the fullpath if it is different from the import path from the source file
						fullpath := ipath
						if mapped, ok := jpkg.ImportMap[ipath]; ok {
							fullpath = mapped
						}

						ok := impMask[fullpath]

						if !ok {
							impMask[fullpath] = true

							if fullpath != "C" {

								ipkg := register(fullpath)

								pkg.Imports[ipath] = ipkg

								ipkg.Active = ipkg.Active || built
							}

							if built {
								found += 1
							}
						}

						// If we are tracking the file imports make sure to add it
						if pkg.FileImports != nil {
							fileImports := pkg.FileImports[file]
							if fileImports == nil {
								fileImports = make(map[string]string, count)
								pkg.FileImports[file] = fileImports
							}

							fileImports[name] = ipath
						}

						if globalName2IName[name] == "" {
							globalName2IName[name] = ipath
						}

					},
				)
				if err != nil {
					return nil, err
				}

				// Sanity checks
				if found != len(jpkg.Imports) {
					panic("parsed imports and go-list imports length mismatch")
				}

				for _, listImpt := range jpkg.Imports {
					if !impMask[listImpt] {
						panic("parsed imports list missing go-list entry")
					}
				}

			} else {
				// We still need to search every import
				for ipath, ipkg := range pkg.Imports {
					pkg.Imports[ipath] = register(ipkg.ImportPath)
				}
			}

			pkg.Parents = make([]*Package, 0, len(pkg.Parents))
			pkg.color = searchable
		}

		for path := range toLoadPkgs {
			next = append(next, path)
		}

		cli = false
	}

	// //// / //
	// PASS 2 //
	// //// / //

	// Perform recursive DFS on the tree
	//
	// During the search:
	// - Build topology
	// - Check for cycles
	// - Build exported types for packages
	var pass2 func(pkg *Package) (int, error)
	pass2 = func(pkg *Package) (int, error) {
		// Handle cases where we have visited the node already
		if pkg.color != searchable {
			switch {
			case pkg.color == searched:
				return pkg.Layer.Level, nil
			case pkg.color == searching:
				return -1, &importCycleError{
					stack: []string{
						pkg.ImportPath,
					},
				}
			default:
				panic("unknown package mark")
			}
		}

		// Mark so that if we see it again, we know we have a cycle
		pkg.color = searching

		level := 0

		// Visit each import path
		for _, ipkg := range pkg.Imports {
			ipkg.Parents = append(ipkg.Parents, pkg)

			seenlevel, err := pass2(ipkg)
			if err != nil {
				// Create traceback for import cycles
				if ice, ok := err.(*importCycleError); ok {
					ice.stack = append(ice.stack, pkg.ImportPath)
				}
				return -1, err
			}

			// If the child's level is higher or identical to our currently known level we move up
			if seenlevel >= level {
				level = seenlevel + 1
			}
		}

		// We now have a known level, so we attach the import layer
		for len(layers) <= level {
			nextLayer := &ProcGroup{
				Level: len(layers),
				Next:  layers[len(layers)-1],
			}
			layers = append(layers, nextLayer)
		}

		layers[level].Packages = append(layers[level].Packages, pkg)
		pkg.Layer = layers[level]

		if pkg.Types == nil {
			if len(pkg.Configs[0].GoFiles) == 0 {
				// If the build list is empty make an empty package
				pkg.Types = types.NewPackage(pkg.ImportPath, pkg.Name)
				pkg.Types.MarkComplete()
			} else {
				// Do a
				syntax := make([]*ast.File, 0, len(pkg.Configs[0].GoFiles))
				for _, gofile := range pkg.Configs[0].GoFiles {
					fpath := filepath.Join(pkg.Dir, gofile.Name)
					fsyn, err := parser.ParseFile(pkg.Fset, fpath, nil, 0)

					if err != nil {
						return -1, fmt.Errorf("unable to parse file %v in package %v: %w", fpath, pkg.ImportPath, err)
					}

					syntax = append(syntax, fsyn)
				}

				imptF := iImporter(func(path string) (*types.Package, error) {
					if path == "unsafe" {
						return types.Unsafe, nil
					}

					ipkg := pkg.Imports[path]
					if ipkg == nil {
						panic("import not found for partial type check")
					}

					return ipkg.Types, nil
				})

				tcfg := &types.Config{
					Importer:                 imptF,
					Error:                    func(err error) {}, // Make not nil so that we can have complete set of export data
					IgnoreFuncBodies:         true,               // Partial type list is only for export data
					DisableUnusedImportCheck: true,
					FakeImportC:              true,
				}

				pkg.Types, _ = tcfg.Check(pkg.ImportPath, pkg.Fset, syntax, nil)
			}
		}

		pkg.color = searched

		PackageImportGraph = layers
		return level, nil
	}

	for _, pkg := range clipkgs {
		switch {
		case pkg.color == searchable:
			// DFS on this node
			_, err := pass2(pkg)
			if err != nil {
				return nil, err
			}
		case pkg.color > searching:
			// Node visited already, pass
			continue
		case pkg.color == searching:
			panic("package is marked visited when no DFS is currently being performed")
		default:
			panic("unknown mark for package")
		}
	}

	return layers[len(layers)-1], nil
}

func loadBuildConfigs(fset *token.FileSet, jpkg *RawPackage, loadAllConfigs bool, onImport func(file string, name string, ipath string, built bool, count int)) ([]BuildConfig, error) {
	var builds []BuildConfig = make([]BuildConfig, 0, 2)

	// TODO: return errors from loading new files and invalidate the build configs
	// let the port controller figure out what to do
	alwaysBuild := make([]*GoFile, 0, len(jpkg.GoFiles)+len(jpkg.CgoFiles))

	platforms := make(
		map[string]*struct {
			hash    uintptr
			gofiles []*GoFile
		},
		len(tags.UNIX_PLATFORM_RANKING),
	)

	// Collapse configs down using hashes and register new ones
	hashes := make(map[uintptr]int, len(tags.UNIX_PLATFORM_RANKING)+1)

	builds = append(builds, BuildConfig{})
	defaultHash := uintptr(0)

	// Read normal go files that are built
	for _, file := range jpkg.GoFiles {
		path := filepath.Join(jpkg.Dir, file)
		cstr, cgo, err := loadHeader(fset, path, func(name, ipath string, count int) {
			onImport(file, name, ipath, true, count)
		})
		if err != nil {
			return nil, err
		}

		if cgo {
			panic("cgo file found when parsing non-cgo files")
		}

		gofile := &GoFile{
			Name: file,
		}

		builds[0].GoFiles = append(builds[0].GoFiles, gofile)

		// Don't check tags for GOROOT packages (see https://github.com/ZOSOpenTools/wharf/issues/7)
		if jpkg.Goroot {
			alwaysBuild = append(alwaysBuild, gofile)
			continue
		}

		switch cnstr := cstr.(type) {
		case tags.All:
			alwaysBuild = append(alwaysBuild, gofile)
		case tags.Supported:
			alwaysBuild = append(alwaysBuild, gofile)
		case tags.Platforms:
			defaultHash += uintptr(unsafe.Pointer(gofile))
			for tag := range cnstr {
				if platforms[tag] == nil {
					platforms[tag] = new(struct {
						hash    uintptr
						gofiles []*GoFile
					})
				}
				platforms[tag].gofiles = append(platforms[tag].gofiles, gofile)
				platforms[tag].hash += uintptr(unsafe.Pointer(gofile))
			}
		case tags.Ignored:
			panic("build never constraint found for actively built go file")
		default:
			panic("invalid build constraint type")
		}
	}

	// Read CGo files that are built
	for _, file := range jpkg.CgoFiles {
		path := filepath.Join(jpkg.Dir, file)
		bc, cgo, err := loadHeader(fset, path, func(name, ipath string, count int) {
			onImport(file, file, ipath, true, count)
		})
		if err != nil {
			return nil, err
		}

		if !cgo {
			panic("non-cgo file found when parsing cgo files")
		}

		gofile := &GoFile{
			Name:  file,
			IsCgo: true,
		}

		builds[0].GoFiles = append(builds[0].GoFiles, gofile)

		switch cnstr := bc.(type) {
		case tags.All:
			alwaysBuild = append(alwaysBuild, gofile)
		case tags.Supported:
			alwaysBuild = append(alwaysBuild, gofile)
		case tags.Platforms:
			defaultHash += uintptr(unsafe.Pointer(gofile))
			for tag := range cnstr {
				if platforms[tag] == nil {
					platforms[tag] = new(struct {
						hash    uintptr
						gofiles []*GoFile
					})
				}
				platforms[tag].gofiles = append(platforms[tag].gofiles, gofile)
				platforms[tag].hash += uintptr(unsafe.Pointer(gofile))
			}
		case tags.Ignored:
			panic("build never constraint found for actively built cgo file")
		default:
			panic("invalid build constraint type")
		}
	}

	hashes[defaultHash] = 0

	if loadAllConfigs {
		for _, file := range jpkg.IgnoredGoFiles {
			path := filepath.Join(jpkg.Dir, file)
			bc, cgo, err := loadHeader(fset, path, func(name, ipath string, count int) {
				onImport(file, name, ipath, false, count)
			})
			if err != nil {
				return nil, err
			}

			gofile := &GoFile{
				Name:  file,
				IsCgo: cgo,
			}

			switch cnstr := bc.(type) {
			case tags.All:
				panic("build always constraint found for ignored file")
			case tags.Supported:
				panic("build for GOOS constraint found for ignored file")
			case tags.Platforms:
				for tag := range cnstr {
					if platforms[tag] == nil {
						platforms[tag] = new(struct {
							hash    uintptr
							gofiles []*GoFile
						})
					}
					platforms[tag].gofiles = append(platforms[tag].gofiles, gofile)
					platforms[tag].hash += uintptr(unsafe.Pointer(gofile))
				}
			case tags.Ignored:
				continue
			default:
				panic("invalid build constraint type")
			}
		}

		// TODO: properly handle hash collisions
		for _, pltf := range tags.UNIX_PLATFORM_RANKING {
			if platforms[pltf] == nil {
				continue
			}

			cfgidx, ok := hashes[platforms[pltf].hash]
			if !ok {
				builds = append(builds, BuildConfig{
					Platforms: []string{pltf},
					GoFiles:   append(platforms[pltf].gofiles, alwaysBuild...),
				})

				hashes[platforms[pltf].hash] = len(builds) - 1
			} else {
				builds[cfgidx].Platforms = append(builds[cfgidx].Platforms, pltf)
			}
		}

	}

	return builds, nil
}

func loadHeader(fset *token.FileSet, path string, onImport func(name string, ipath string, count int)) (bc tags.Constraint, cgo bool, err error) {
	var src []byte

	bc = tags.Ignored{}

	src, err = os.ReadFile(path)
	if err != nil {
		return
	}

	bc = tags.Parse(path, src, base.GOOS(), base.BuildTags)
	if _, ok := bc.(tags.Ignored); ok {
		return bc, false, nil
	}

	var syntax *ast.File
	syntax, err = parser.ParseFile(fset, path, src, parser.ImportsOnly)
	for _, isyn := range syntax.Imports {
		// String literals are wrapped in double quotes, need to remove those
		ipath := strings.TrimPrefix(strings.TrimSuffix(isyn.Path.Value, "\""), "\"")
		if ipath == "C" {
			cgo = true
		}

		var name string
		if isyn.Name != nil {
			name = isyn.Name.Name
		} else {
			name = genpath.Base(ipath)
			dotidx := strings.Index(name, ".")
			if dotidx > -1 {
				name = name[:dotidx]
			}
			dashidx := strings.LastIndex(name, "-")
			if dashidx > -1 {
				name = name[dashidx+1:]
			}
		}

		onImport(name, ipath, len(syntax.Imports))
	}

	return
}

func (pkg *Package) LoadSyntax() (err error) {
	for idx := range pkg.Configs {
		cfg := &pkg.Configs[idx]
		cfg.Syntax = make([]*ast.File, 0, len(cfg.GoFiles))
		for _, gofile := range cfg.GoFiles {
			syntax := gofile.Syntax
			if syntax == nil {
				var src []byte
				src, err = os.ReadFile(filepath.Join(pkg.Dir, gofile.Name))
				if err != nil {
					return
				}
				syntax, err = parser.ParseFile(pkg.Fset, gofile.Name, src, 0)
				if err != nil {
					return
				}
				gofile.Syntax = syntax
			}

			cfg.Syntax = append(cfg.Syntax, syntax)
		}
	}

	return
}

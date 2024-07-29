// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package pkg2

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/zosopentools/wharf/internal/base"
	"github.com/zosopentools/wharf/internal/tags"
	"github.com/zosopentools/wharf/internal/util"
)

var debugPath string

func lpanic(msg string) {
	panic(fmt.Sprintf("%v: %v", debugPath, msg))
}

var cache map[string]*Package = make(map[string]*Package, 50)

// Use go-list to load all packages and build the initial tree
//
// We load as many packages as we can at once - and pick up any "unimported" packages
// (packages that were imported by source files not marked for build under the present system)
// Any unimported packages we then go and load ourselves (and continue this process until all packages are loaded)
func List(paths []string) (ImportTree, error) {
	// fmt.Fprintf(os.Stderr, "\n#### LOAD #### \n\n")
	found := make(map[string]*Package, len(cache))
	seeking := make(map[string]bool, 10)

	next := paths
	firstLoad := true
	matching := make([]*Package, 0, len(paths))

	identify := func(path string) *Package {
		pkg := cache[path]
		if pkg == nil {
			// fmt.Fprintln(os.Stderr, path)
			pkg = &Package{}
			cache[path] = pkg
		}
		return pkg
	}

	for len(next) > 0 {
		listout, err := util.GoList(next)
		if err != nil {
			return ImportTree{}, err
		}

		var metaPkgs []*MetaPackage
		decoder := json.NewDecoder(strings.NewReader(listout))
		for decoder.More() {
			// Possibility that the package list resolved to only a single package
			metaPkgs = append(metaPkgs, &MetaPackage{})
			if err := decoder.Decode(metaPkgs[len(metaPkgs)-1]); err != nil {
				// TODO: provide better error
				panic(err)
			}
		}

		if len(metaPkgs) == 0 {
			return ImportTree{}, fmt.Errorf("no packages found in the workspace")
		}

		for _, meta := range metaPkgs {
			debugPath = meta.ImportPath
			if seeking[meta.ImportPath] {
				delete(seeking, meta.ImportPath)
			}
			if found[meta.ImportPath] != nil {
				if !meta.DepOnly {
					lpanic("loaded a package more than once in the same pass")
				}
				continue
			}

			pkg := identify(meta.ImportPath)
			pkg.FirstLoad = pkg.Meta == nil
			doLoad := pkg.FirstLoad || pkg.Meta.Dir != meta.Dir
			pkg.Dirty = doLoad
			pkg.Meta = meta
			pkg.Included = firstLoad
			pkg.Modified = pkg.modified
			pkg.modified = false
			pkg.DepDirty = false

			if firstLoad && !meta.DepOnly {
				if meta.Module == nil || !meta.Module.Main {
					return ImportTree{}, fmt.Errorf("%v: target package must be included in Main module", meta.ImportPath)
				}
				matching = append(matching, pkg)
			}

			// go-list errors mean the environment is bad -> stop loading for bad environments
			if meta.Error != nil {
				// if the package does not contain any go file can be build on the platform
				if IsExcludeGoListError(meta.Error.Err) {
					// since the file will be ignore go list does not read any of the go file
					// It can not find a package name. Need to find the package manually
					if meta.Name == "" {
						meta.Name = tags.FindPackageName(meta.Dir, meta.IgnoredGoFiles)
					}
				} else {
					return ImportTree{}, fmt.Errorf("unable to load %v: %v", meta.ImportPath, meta.Error.Err)
				}
			}

			// Use the directory as a check for if package information has changed
			// Go uses different directories for different module versions
			if doLoad {
				// fmt.Fprintf(os.Stderr, "\n# %v\n", pkg.Meta.ImportPath)
				if err = loadPkg(pkg); err != nil {
					return ImportTree{}, err
				}

				// Register all imported packages (and do sanity check on packages that go-list reported)
				touchedIPaths := make(map[string]bool, len(pkg.Meta.Imports))
				iCount := 0
				for _, file := range pkg.Files {
					for _, iPath := range file.Imports {
						// fmt.Fprintf(os.Stderr, "in file %v found %v\n", file.Name, iPath)
						trueIPath := iPath
						if mapped, ok := pkg.Meta.ImportMap[iPath]; ok {
							trueIPath = mapped
						}

						inc, ok := touchedIPaths[trueIPath]
						if !ok {
							touchedIPaths[trueIPath] = false
							if trueIPath != CGO_PACKAGE_NAME {
								pkg.Imports[iPath] = identify(trueIPath)
							}
						}

						if !inc && file.Default {
							touchedIPaths[trueIPath] = true
							iCount++
						}
					}

					for _, iPath := range file.AnonImports {
						// fmt.Fprintf(os.Stderr, "in file %v found %v\n", file.Name, iPath)
						trueIPath := iPath
						if mapped, ok := pkg.Meta.ImportMap[iPath]; ok {
							trueIPath = mapped
						}

						inc, ok := touchedIPaths[trueIPath]
						if !ok {
							touchedIPaths[trueIPath] = false
							if trueIPath != CGO_PACKAGE_NAME {
								pkg.Imports[iPath] = identify(trueIPath)
							}
						}

						if !inc && file.Default {
							touchedIPaths[trueIPath] = true
							iCount++
						}
					}
				}

				if iCount != len(pkg.Meta.Imports) {
					lpanic(fmt.Sprintf("parsed imports and go-list imports length mismatch: found %v wanted %v", iCount, len(pkg.Meta.Imports)))
				}

				for _, iPath := range pkg.Meta.Imports {
					if !touchedIPaths[iPath] {
						lpanic(fmt.Sprintf("parsed imports list missing go-list entry: %v", iPath))
					}
				}
			}

			found[meta.ImportPath] = pkg
			for iPath := range pkg.Imports {
				if mapped, ok := pkg.Meta.ImportMap[iPath]; ok {
					iPath = mapped
				}
				if found[iPath] == nil {
					seeking[iPath] = true
				}
			}
		}

		firstLoad = false
		next = make([]string, 0, len(seeking))
		for path := range seeking {
			next = append(next, path)
		}
	}

	return ImportTree{from: matching}, nil
}

func loadPkg(pkg *Package) error {
	var debugFile string
	fpanic := func(msg string) {
		lpanic(fmt.Sprintf("%v: %v", debugFile, msg))
	}
	pkg.Builds = make([]BuildConfig, 0, 2)
	pkg.Files = make(map[string]*GoFile, len(pkg.Meta.GoFiles)+len(pkg.Meta.CgoFiles)+len(pkg.Meta.IgnoredGoFiles))
	pkg.Imports = make(map[string]*Package, len(pkg.Meta.Imports))

	if b := backupLookupMap[pkg.Meta.Name]; b == nil {
		backupLookupMap[pkg.Meta.Name] = pkg
	}

	// TODO: return errors from loading new files and invalidate the build configs
	// let the port controller figure out what to do
	alwaysBuild := make([]*GoFile, 0, len(pkg.Meta.GoFiles)+len(pkg.Meta.CgoFiles))

	platforms := make(
		map[string]*struct {
			hash  uint64
			files []*GoFile
		},
		len(tags.UNIX_PLATFORM_RANKING),
	)

	// Collapse configs down using hashes and register new ones
	hashes := make(map[uint64]int, len(tags.UNIX_PLATFORM_RANKING)+1)
	nextHash := uint64(1)
	hashCheck := 0

	getHash := func() uint64 {
		hashCheck += 1
		if hashCheck >= 64 {
			lpanic("too many hashes")
		}

		hash := nextHash
		nextHash = nextHash << 1

		return hash
	}

	pkg.Builds = append(pkg.Builds, BuildConfig{})
	defaultHash := uint64(0)

	isStd := IsStdlibPkg(pkg)

	// Read normal go files that are built
	for _, fname := range pkg.Meta.GoFiles {
		debugFile = fname
		file := &GoFile{
			Name:    fname,
			Path:    filepath.Join(pkg.Meta.Dir, fname),
			Default: true,
		}
		pkg.Files[fname] = file
		if err := loadGoFile(file, FileSet, true, isStd); err != nil {
			return err
		}

		if file.Cgo {
			fpanic("cgo file found when parsing non-cgo files")
		}

		pkg.Builds[0].Files = append(pkg.Builds[0].Files, file)
		pkg.Builds[0].Syntax = append(pkg.Builds[0].Syntax, file.Syntax)

		// Don't check tags for GOROOT packages (see https://github.com/ZOSOpenTools/wharf/issues/7)
		if isStd {
			alwaysBuild = append(alwaysBuild, file)
			continue
		}

		switch cnstr := file.Tags.(type) {
		case tags.All:
			alwaysBuild = append(alwaysBuild, file)
		case tags.Supported:
			alwaysBuild = append(alwaysBuild, file)
		case tags.Platforms:
			hash := getHash()
			defaultHash += hash
			for tag := range cnstr {
				if platforms[tag] == nil {
					platforms[tag] = new(struct {
						hash  uint64
						files []*GoFile
					})
				}
				platforms[tag].files = append(platforms[tag].files, file)
				platforms[tag].hash += hash
			}
		case tags.Ignored:
			fpanic("build never constraint found for actively built go file")
		default:
			fpanic("invalid build constraint type")
		}
	}

	// Read CGo files that are built
	for _, fname := range pkg.Meta.CgoFiles {
		debugFile = fname
		file := &GoFile{
			Name:    fname,
			Path:    filepath.Join(pkg.Meta.Dir, fname),
			Default: true,
		}
		pkg.Files[fname] = file
		if err := loadGoFile(file, FileSet, true, isStd); err != nil {
			return err
		}

		if !file.Cgo {
			fpanic("non-cgo file found when parsing cgo files")
		}

		pkg.Builds[0].Files = append(pkg.Builds[0].Files, file)
		pkg.Builds[0].Syntax = append(pkg.Builds[0].Syntax, file.Syntax)

		// Don't check tags for GOROOT packages (see https://github.com/ZOSOpenTools/wharf/issues/7)
		if isStd {
			alwaysBuild = append(alwaysBuild, file)
			continue
		}

		switch cnstr := file.Tags.(type) {
		case tags.All:
			alwaysBuild = append(alwaysBuild, file)
		case tags.Supported:
			alwaysBuild = append(alwaysBuild, file)
		case tags.Platforms:
			hash := getHash()
			defaultHash += hash
			for tag := range cnstr {
				if platforms[tag] == nil {
					platforms[tag] = new(struct {
						hash  uint64
						files []*GoFile
					})
				}
				platforms[tag].files = append(platforms[tag].files, file)
				platforms[tag].hash += hash
			}
		case tags.Ignored:
			fpanic("build never constraint found for actively built cgo file")
		default:
			fpanic("invalid build constraint type")
		}
	}

	if !IsStdlibPkg(pkg) {
		for _, fname := range pkg.Meta.IgnoredGoFiles {
			debugFile = fname
			file := &GoFile{
				Name: fname,
				Path: filepath.Join(pkg.Meta.Dir, fname),
			}
			pkg.Files[fname] = file
			if err := loadGoFile(file, FileSet, false, false); err != nil {
				return err
			}

			switch cnstr := file.Tags.(type) {
			case tags.All:
				fpanic("build always constraint found for ignored file")
			case tags.Supported:
				fpanic("build for GOOS constraint found for ignored file")
			case tags.Platforms:
				hash := getHash()
				for tag := range cnstr {
					if platforms[tag] == nil {
						platforms[tag] = new(struct {
							hash  uint64
							files []*GoFile
						})
					}
					platforms[tag].files = append(platforms[tag].files, file)
					platforms[tag].hash += hash
				}
			case tags.Ignored:
				continue
			default:
				fpanic("invalid build constraint type")
			}
		}

		// Build the actual builds list
		hashes[defaultHash] = 0
		for _, pltf := range tags.UNIX_PLATFORM_RANKING {
			if platforms[pltf] == nil {
				continue
			}

			cfgidx, ok := hashes[platforms[pltf].hash]
			if !ok {
				pkg.Builds = append(pkg.Builds, BuildConfig{
					Platforms: []string{pltf},
					Files:     append(platforms[pltf].files, alwaysBuild...),
				})

				hashes[platforms[pltf].hash] = len(pkg.Builds) - 1
			} else {
				pkg.Builds[cfgidx].Platforms = append(pkg.Builds[cfgidx].Platforms, pltf)
			}
		}

	}

	return nil
}

func loadGoFile(file *GoFile, fset *token.FileSet, syntax bool, forceLoad bool) error {
	src, err := os.ReadFile(file.Path)
	if err != nil {
		return err
	}

	file.Tags = tags.Parse(file.Name, src, base.GOOS(), base.BuildTags)
	if _, ok := file.Tags.(tags.Ignored); ok && !forceLoad {
		return nil
	}

	mode := (parser.Mode)(0)
	if !syntax {
		mode = parser.ImportsOnly
	}

	parsed, err := parser.ParseFile(fset, file.Name, src, mode)
	if err != nil {
		return err
	}

	if syntax {
		file.Syntax = parsed
	}

	altNames := make(map[string]string)
	file.Imports = make(map[string]string, len(parsed.Imports))
	for _, isyn := range parsed.Imports {
		// String literals are wrapped in double quotes, need to remove those
		ipath := strings.TrimPrefix(strings.TrimSuffix(isyn.Path.Value, "\""), "\"")
		if ipath == CGO_PACKAGE_NAME {
			file.Cgo = true
		}

		var name string
		if isyn.Name != nil {
			name = isyn.Name.Name
			if name == "_" || name == "." {
				file.AnonImports = append(file.AnonImports, ipath)
				continue
			}
		} else {
			aname, alt := ImportPathToAssumedName(ipath)
			name = aname
			if alt != "" {
				altNames[ipath] = alt
			}
		}
		if conflict := file.Imports[name]; conflict != "" {
			if alt := altNames[ipath]; alt != "" {
				name = alt
			} else if alt := altNames[conflict]; alt != "" {
				// Write current package
				file.Imports[ipath] = name
				// Setup to write package we just overrode
				ipath = conflict
				name = alt
			}
		}
		if file.Imports[name] != "" {
			lpanic(fmt.Sprintf("%v: duplicate import name %v: (%v, %v)", file.Name, name, file.Imports[name], ipath))
		}
		file.Imports[name] = ipath
	}

	return nil
}

# Wharf

A simple way to build and maintain ports of Go packages that everyone can contribute to

## What is it?

Wharf automatically tries to fix build errors caused by packages that don't support z/OS
by investigating type checking errors and updating module dependencies/retagging files to include
missing function, varialble, constant, type definitions.

This program can:
* Port existing packages in a workspace
* Port dependencies of modules (dependencies will be moved inside workspace)
* Provide insights into/solutions to possible issues with package
* Make edits and run commands to ensure the package can be installed on z/OS without error

## Installation

### Go Install

```
go install github.com/zosopentools/wharf
```

### From source

```
git clone git@github.com:zosopentools/wharf && cd wharf
go install
```

## Usage

Run it similarly to `go build`.

`wharf [-n] [-v] [-t] [-q] [-d] [-f] [-tags] <packages>`

Currently wharf only supports executing within a workspace (which means operating similarly to `go build -mod=readonly`)

- Must be inside a Go workspace (initialize one in your current folder by running `go work init`)
- Package to port must exist inside workspace

### Flags

**-n**
Dry-run mode; disables edits, script will only make suggestions

**-v**
Enable verbose output

**-t**
Run unit tests found in packages that were altered and output their result (ignored if in dry-run mode)

**-q**
Clone ported dependencies from VCS instead of copying from module cache (keeps VCS information)

**-d**
Base path to clone imported modules to

**-f**
Force operation even in unsafe situations (such as imported module path already existing) - useful for scripts

### Example

#### Set up workspace

```
mkdir wharf-work && cd wharf-work
go work init
```

#### Clone module to port

```
git clone git@github.com/prometheus/prometheus
go work use ./prometheus
```

#### Run Wharf
```
wharf ./prometheus/cmd/...
```

#### Install ported packages
```
go install ./prometheus/cmd/...
```

## Understanding the Porting Process

### Main Process

Wharf works by first loading the environment using `go list` and gathering information related to the package structures.
This occurs in `internal/packages`, which acts almost as a special purpose compiler front-end implementation.

After the load occurs type-checking occurs. Packages are type checked until the first error occurs. At which the porting process begins for that package.

Packages are ported based on the structure of the dependency tree. Packages that are higher up in the dependency tree (have fewer levels of sub-dependencies).

### Porting packages

When we port a package we check what the error is, if it is a missing definition we proceed with porting.
If some other type checking error occurs we stop porting that package.

Porting follows these steps:
1. Attempts to update the module that contains the package (if it is not a main module)
2. Change the build tags of the files to include any definitions that are missing such that:
 - Dependents of the package can be built
 - The package itself (barring issues with dependencies) can be built
3. Port any dependencies that we are missing definitions from
4. Retag to remove any definitions that are expected from dependencies, but that we could not include in the build
5. If any dependency definitions are left over try and see if we have code to replace them specifically

This process works because:

After attempting to update a module, if the module has any packages that contain errors we naively revert back to the original version of the module that was used. Therefore we lock in the version of the source code we use. Go also ensures that there can never be import cycles in code, therefore it is impossible that trying to fix a package further down in the dependency graph will impact a package higher up in the chain.

In otherwords - once we begin get past step 1 for a package we will know for a fact that the dependency graph will not change at that level or above, and we can safely work on it.

### Planned Features

- Better CGo support
- Support for workspace-less environments

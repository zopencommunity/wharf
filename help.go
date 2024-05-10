// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
package main

var helpText = `
The wharf command builds packages
making changes to the target package and any dependencies
so that the package can successfully build on IBM z/OS.
Outputs actions taken to 'gozos-port.log'.

To run, working directory must be inside a Go workspace.

Usage:
	wharf [flags] <package>

Options:
-help
	Display this message
-n
	Don't make changes, just print out suggested actions, cannot run with -t
-v
	Verbose output (will still get logged to output file)
-t
	Run tests on the package after successful build, cannot run with -n
-q <path>
	Clones dependencies from VCS instead of copying from module cache
-p
	REQUIRES -q
	Automatically create and save patch files (diffs)
-config
	Path to config for additional code edits
-d
	Filesystem pat to store imported modules
-f
	Force apply changes
-version
	Display version information
`

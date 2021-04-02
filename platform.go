// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type PlatformSpecific struct {
	objsuffix   string
	exesuffix   string
	libprefix   string
	libsuffix   string
	dllprefix   string
	dllsuffix   string
	installfile func(filename, destdir string, filemode os.FileMode) error
}

var (
	windowsPlatform = PlatformSpecific{
		objsuffix:   ".obj",
		exesuffix:   ".exe",
		libprefix:   "",
		libsuffix:   ".lib",
		dllprefix:   "",
		dllsuffix:   ".dll",
		installfile: installByCopyingFile,
	}
	macosPlatform = PlatformSpecific{
		objsuffix:   ".o",
		exesuffix:   "",
		libprefix:   "lib",
		libsuffix:   ".a",
		dllprefix:   "lib",
		dllsuffix:   ".dylib",
		installfile: installWithUsrBinInstall,
	}
	elfPlatform = PlatformSpecific{
		objsuffix:   ".o",
		exesuffix:   "",
		libprefix:   "lib",
		libsuffix:   ".a",
		dllprefix:   "lib",
		dllsuffix:   ".so",
		installfile: installWithUsrBinInstall,
	}
)

var (
	// The PlatformSpecific for the build host.
	//
	platform *PlatformSpecific

	// This matches platforms **other** than this one. This is
	// used to ignore files using Go-style platform-specific
	// filenames.
	//
	otherPlatformNamesRegexp *regexp.Regexp
)

func init() {
	platforms := []string{
		"aix",
		"darwin",
		"dragonfly",
		"freebsd",
		"illumos",
		"ios",
		"linux",
		"netbsd",
		"openbsd",
		"solaris",
		"windows",
	}

	switch runtime.GOOS {
	case "windows":
		platform = &windowsPlatform
	case "darwin":
		platform = &macosPlatform
	default:
		platform = &elfPlatform
	}

	var otherPlatformNames []string
	for _, name := range platforms {
		if name != runtime.GOOS {
			otherPlatformNames = append(otherPlatformNames, name)
		}
	}
	otherPlatformNamesRegexp = regexp.MustCompile("_(" + strings.Join(otherPlatformNames, "|") + ")\\.")
}

func (p *PlatformSpecific) LibFilename(path string) string {
	return formFilename(p.libprefix, path, p.libsuffix)
}

func (p *PlatformSpecific) DllFilename(path string) string {
	return formFilename(p.dllprefix, path, p.dllsuffix)
}

func (p *PlatformSpecific) ExeFilename(path string) string {
	return formFilename("", path, p.exesuffix)
}

func (p *PlatformSpecific) ObjFilename(path string) string {
	return formFilename("", path, p.objsuffix)
}

func formFilename(prefix, path, suffix string) string {
	dirname, basename := filepath.Dir(path), filepath.Base(path)
	if prefix != "" && !strings.HasPrefix(basename, prefix) {
		basename = prefix + basename
	}
	if suffix != "" && !strings.HasSuffix(basename, suffix) {
		basename += suffix
	}
	return filepath.Clean(filepath.Join(dirname, basename))
}

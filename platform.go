// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
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
	allPlatforms := []string{
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
	for _, name := range allPlatforms {
		if name != runtime.GOOS {
			otherPlatformNames = append(otherPlatformNames, name)
		}
	}
	otherPlatformNamesRegexp = regexp.MustCompile("_(" + strings.Join(otherPlatformNames, "|") + ")\\.")
}

func (p *PlatformSpecific) libFilename(path string) string {
	return formFilename(p.libprefix, path, p.libsuffix)
}

func (p *PlatformSpecific) dllFilename(path string) string {
	return formFilename(p.dllprefix, path, p.dllsuffix)
}

func (p *PlatformSpecific) exeFilename(path string) string {
	return formFilename("", path, p.exesuffix)
}

func (p *PlatformSpecific) objFilename(path string) string {
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

func installWithUsrBinInstall(filename, destdir string, filemode os.FileMode) error {
	args := []string{"-c", "-m", fmt.Sprintf("%o", int(filemode)), filename, filepath.Join(destdir, filename)}
	if *debug {
		log.Printf("RUN: /usr/bin/install %v", args)
	}
	cmd := exec.Command("/usr/bin/install", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	return cmd.Run()
}

func installByCopyingFile(filename, destdir string, filemode os.FileMode) error {
	dstFilename := filepath.Join(destdir, filename)
	if *debug {
		log.Printf("COPY: %q -> %q", filename, dstFilename)
	}
	src, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filemode)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err != nil {
		dst.Close()
		os.Remove(dstFilename)
	}
	return dst.Close()
}

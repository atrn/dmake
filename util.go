// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// File name extensions for the different languages.
	//
	languageExtension = map[Language][]string{
		CplusplusLanguage:    {"*.cpp", "*.cc", "*.cxx", "*.c++"},
		CLanguage:            {"*.c"},
		ObjcLanguage:         {"*.m"},
		ObjcplusplusLanguage: {"*.mm"},
	}

	// Regular expression to match a definition of a, well-formed,
	// C/C++ main() function.
	//
	//	int main()
	//	int main(void)
	//	int main(int
	//
	mainFunctionRegexp = regexp.MustCompile("^[ \t]*(func|int)?[ \t]*main[ \t]*\\((void|int|)")
)

func Getenv(name, defaultValue string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return defaultValue
}

func AddDetail(err error, format string, args ...interface{}) error {
	return fmt.Errorf("%s (%s)", err, fmt.Sprintf(format, args...))
}

func DefinesMain(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		log.Print(err)
		return false
	}
	defer file.Close()
	for scanner := bufio.NewScanner(file); scanner.Scan(); {
		line := scanner.Text()
		if mainFunctionRegexp.MatchString(line) {
			return true
		}
	}
	return false
}

func ObjectFilename(srcfile string, objsdir string) string {
	dirname, basename := filepath.Dir(srcfile), filepath.Base(srcfile)
	path := filepath.Clean(filepath.Join(filepath.Join(dirname, objsdir), basename))
	return platform.ObjFilename(strings.TrimSuffix(path, filepath.Ext(basename)))
}

func DependenciesFilename(ofile string, depsdir string) string {
	dirname, basename := filepath.Dir(ofile), filepath.Base(ofile)
	if strings.HasSuffix(dirname, objsdir) {
		return filepath.Join(dirname, basename)
	} else {
		return filepath.Join(dirname, depsdir, basename)
	}
}

func Glob(pattern string) (filenames []string, matched bool, err error) {
	var matches []string
	matches, err = filepath.Glob(pattern)
	if err != nil {
		return
	}
	if len(matches) > 0 {
		filenames = make([]string, 0, len(matches))
		for _, name := range matches {
			if otherPlatformNamesRegexp.MatchString(name) {
				if *debugFlag {
					log.Printf("DEBUG: glob ignoring %q", name)
				}
				continue
			}
			filenames = append(filenames, name)
		}
	}
	matched = len(filenames) > 0
	return
}

func ExpandGlobs(patterns string) ([]string, error) {
	var filenames []string
	for _, pattern := range strings.Fields(patterns) {
		if names, matched, err := Glob(pattern); err != nil {
			return nil, err
		} else if matched {
			filenames = append(filenames, names...)
		}
	}
	return filenames, nil
}

func CreateFile(path string, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	fmt.Fprint(file, content)
	err = file.Close()
	if err != nil {
		os.Remove(path)
	}
	return err
}

func SourceFiles() ([]string, Language, error) {
	for lang, patterns := range languageExtension {
		for _, pattern := range patterns {
			paths, matches, err := Glob(pattern)
			if err != nil {
				return nil, UnknownLanguage, err
			}
			if matches {
				if langflag != UnknownLanguage {
					lang = langflag
				}
				return paths, lang, nil
			}
		}
	}
	return nil, UnknownLanguage, nil
}

func FilenameForType(outputtype OutputType, name string) string {
	switch outputtype {
	case DllOutputType:
		return platform.DllFilename(name)
	case PluginOutputType:
		return platform.PluginFilename(name)
	case ExeOutputType:
		return platform.ExeFilename(name)
	case LibOutputType:
		return platform.LibFilename(name)
	default:
		panic("unexpected outputtype: " + outputtype.String())
	}
}

type CwdRestorer struct {
	path string
	err  error
}

func ChangeDirectory(path string) (CwdRestorer, error) {
	r := CwdRestorer{}
	r.path, r.err = os.Getwd()
	if r.err != nil {
		r.err = AddDetail(r.err, "os.Getwd")
		return r, r.err
	}
	if r.err = os.Chdir(path); r.err != nil {
		r.err = AddDetail(r.err, "os.Chdir %q", path)
	}
	return r, r.err
}

func (r CwdRestorer) Restore() {
	if r.err == nil {
		r.err = os.Chdir(r.path)
		if r.err != nil {
			r.err = AddDetail(r.err, "os.Chdir %q", r.path)
		}
	}
}

func installWithUsrBinInstall(filename, destdir string, filemode os.FileMode) error {
	args := []string{"-c", "-m", fmt.Sprintf("%o", int(filemode)), filename, filepath.Join(destdir, filename)}
	if *debugFlag {
		log.Printf("RUN: /usr/bin/install %v", args)
	}
	cmd := exec.Command("/usr/bin/install", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	return cmd.Run()
}

func installByCopyingFile(filename, destdir string, filemode os.FileMode) error {
	dstFilename := filepath.Join(destdir, filename)
	if *debugFlag {
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

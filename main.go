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
	"flag"
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

const (
	srcsfileFilename  = "SRCS"
	dmakefileFilename = ".dmake"
	defaultDepsDir    = ".dcc.d"
	defaultObjsDir    = ".dmake.o"
)

var (
	srcs     []string // names of the source files
	kind     = ""     // dcc option "--dll" | "--exe" | "--lib"
	output   = ""     // output filename
	cleaning = false  // true if cleaning

	debug   = flag.Bool("debug", false, "Enable debug output.")
	Cflag   = flag.String("C", "", "Change directory to `directory` before doing anything.")
	oflag   = flag.String("o", "", "Define output `filename`.")
	vflag   = flag.Bool("v", false, "Issue messages.")
	kflag   = flag.Bool("k", false, "Keep going. Don't stop on error.")
	dllflag = flag.Bool("dll", false, "Automatic dynamic, not static, libraries.")

	depsdir = Getenv("DCCDEPS", defaultDepsDir)
	objsdir = Getenv("OBJDIR", defaultObjsDir)

	// Matches platforms other than this one. Used to filter *out*
	// Go-style platform-specific filenames from the contents of a
	// directory.
	//
	otherPlatforms *regexp.Regexp

	// Really should make this stricter, e.g. match any of the following
	// and their forms where the return type is on a separate line and
	// allow for arbitrary whitespace of course.
	//
	//	int main()
	//	int main(void)
	//	int main(int
	//
	// But this works for me...
	//
	mainregex = regexp.MustCompile("^[ \t]*(int)?[ \t]*main\\([^;]*")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("dmake: ")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: dmake [options] {exe|lib|dll} [clean]")
		fmt.Fprintln(os.Stderr, "       dmake [options] path...")
		fmt.Fprintln(os.Stderr, `
The first form builds or cleans the specified module type located
in the current directory.

The second form runs dmake in each of the named directories in sequence,
in this mode the module type cannot be defined on the command line.`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *Cflag != "" {
		err := os.Chdir(*Cflag)
		FatalIf(err)
	}

	cwd, err := os.Getwd()
	FatalIf(err)

	output = filepath.Base(cwd)
	if IsSourceCodeDirectory(output) {
		output = filepath.Base(filepath.Dir(cwd))
	}

	if *oflag == "" {
		*oflag = output
	}

	dirs := make([]string, 0)
	if narg := flag.NArg(); narg > 0 {
		usage := func() {
			flag.Usage()
			os.Exit(1)
		}
		checkclean := func() bool {
			if narg < 2 {
				return false
			} else if narg > 2 || flag.Arg(1) != "clean" {
				usage()
			}
			return true
		}
		switch flag.Arg(0) {
		case "clean":
			cleaning = true
			if narg > 1 {
				usage()
			}
		case "dll":
			kind, output = "--dll", DllFilename(*oflag)
			cleaning = checkclean()
		case "exe":
			kind, output = "--exe", ExeFilename(*oflag)
			cleaning = checkclean()
		case "lib":
			kind, output = "--lib", LibFilename(*oflag)
			cleaning = checkclean()
		default:
			dirs = append(dirs, flag.Args()...)
		}
	}

	if len(dirs) == 0 {
		err := RunDmake(*oflag)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, dir := range dirs {
		if err := RunDmakeIn(dir); err != nil {
			log.Println(err)
			if !*kflag {
				os.Exit(1)
			}
		}
	}
}

// Platform-specific file naming stuff.
//
type extensions struct {
	objsuffix string
	exesuffix string
	libprefix string
	libsuffix string
	dllsuffix string
}

var (
	win = extensions{".obj", ".exe", "", ".lib", ".dll"}
	mac = extensions{".o", "", "lib", ".a", ".dylib"}
	elf = extensions{".o", "", "lib", ".a", ".so"}
	ext *extensions
)

type Vars map[string]string

func init() {
	if runtime.GOOS == "windows" {
		ext = &win
	} else if runtime.GOOS == "darwin" {
		ext = &mac
	} else { // assume linux or bsd
		ext = &elf
	}

	platforms := []string{
		"darwin",
		"freebsd",
		"linux",
		"netbsd",
		"openbsd",
		"windows",
		"solaris",
	}
	var names []string
	for _, name := range platforms {
		if name != runtime.GOOS {
			names = append(names, name)
		}
	}
	otherPlatforms = regexp.MustCompile("_(" + strings.Join(names, "|") + ")\\.")
}

// RunDmakeIn runs dmake in the named directory.
//
func RunDmakeIn(dir string) (err error) {
	oldcwd, err := os.Getwd()
	if err != nil {
		return
	}
	err = os.Chdir(dir)
	if err != nil {
		return
	}
	if *vflag {
		log.Println("entering directory", dir)
	}
	cwd, err2 := os.Getwd()
	if err2 != nil {
		return err2
	}
	kind = ""
	output = filepath.Base(cwd)
	err = RunDmake(filepath.Base(cwd))
	if *vflag {
		log.Println("leaving directory", dir)
	}
	err2 = os.Chdir(oldcwd)
	if err == nil {
		err = err2
	}
	return
}

// RunDmake
//
func RunDmake(opath string) (err error) {


	var havefiles bool

	if dmakefile, err := os.Open(dmakefileFilename); err == nil {
		err = LoadDmakefile(dmakefile, dmakefileFilename)
		dmakefile.Close()
		if err != nil {
			return err
		}
		havefiles = len(srcs) > 0
	}

	if !havefiles {
		var srcsfile *os.File
		if srcsfile, err = os.Open(srcsfileFilename); err == nil {
			srcs, err = ReadSrcs(srcsfile)
			if err != nil {
				return
			}
			if len(srcs) < 1 {
				return fmt.Errorf("%s: file defines no sources", srcsfileFilename)
			}
			havefiles = true
			srcsfile.Close()
			if err != nil {
				return
			}
		} else if !os.IsNotExist(err) {
			return
		}
	}
	if !havefiles {
		srcs, havefiles = MatchFiles("*.cpp")
	}
	if !havefiles {
		srcs, havefiles = MatchFiles("*.cc")
	}
	if !havefiles {
		srcs, havefiles = MatchFiles("*.c")
	}
	if !havefiles {
		return fmt.Errorf("no C or C++ source files found")
	}
	if *debug {
		log.Printf("DEBUG: srcs=%v", srcs)
	}

	// No module type defined, determine executable or library, look for main().
	//
	if kind == "" {
		for _, path := range srcs {
			if FileDefinesMain(path) {
				kind, output = "--exe", ExeFilename(opath)
				break
			}
		}
		if kind == "" {
			if *dllflag {
				kind, output = "--dll", DllFilename(opath)
			} else {
				kind, output = "--lib", LibFilename(opath)
			}
		}
		if *debug {
			log.Printf("DEBUG: inferred module type %q with name %q", kind, output)
		}
	}

	if cleaning {
		os.Remove(output)
		for _, path := range srcs {
			clean := func(path string, deletable string) {
				os.Remove(path)
				dir := filepath.Dir(path)
				if filepath.Base(dir) == deletable {
					os.RemoveAll(dir)
				}
			}
			ofile := ObjectFile(path)
			clean(ofile, objsdir)
			clean(DepsFile(ofile), depsdir)
		}
		return nil
	}

	args := make([]string, 0, 5+len(srcs))
	if *debug {
		args = append(args, "--debug")
	}
	args = append(args, kind, output)
	args = append(args, "--objdir", objsdir)
	args = append(args, srcs...)
	cmd := exec.Command("dcc", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	os.MkdirAll(objsdir, 0777)
	err = cmd.Run()
	return
}

// LoadDmakefile reads the dmakefile and looks for the
// standard variables:
//
//	SRCS	glob pattern matching source files
//	DLL	output a dynamic lib with the defined name
//	LIB	output a static lib with the defined name
//	EXE	output an executable with the defined name
//
func LoadDmakefile(dmakefile *os.File, path string) error {
	vars, err := ReadDmakefile(dmakefile, path)
	if err != nil {
		return err
	}
	var patterns string
	var found bool
	patterns, found = vars["SRCS"]
	if found {
		srcs, err = Expand(patterns, srcs)
		if err != nil {
			return err
		}
		if len(srcs) < 1 {
			return fmt.Errorf("SRCS=%s matches no source files", patterns)
		}
	}

	checkVar := func(varname, kindstr string, fn func(string) string) error {
		if name, exists := vars[varname]; exists {
			if kind != "" && kind != kindstr {
				return fmt.Errorf("%s definition conflicts with %s", varname, kind)
			}
			kind, output = kindstr, fn(name)
		}
		return nil
	}

	if err = checkVar("DLL", "--dll", DllFilename); err != nil {
		return err
	}
	if err = checkVar("EXE", "--exe", ExeFilename); err != nil {
		return err
	}
	if err = checkVar("LIB", "--lib", LibFilename); err != nil {
		return err
	}
	return nil
}

// MatchFiles expands a glob-pattern to locate source files
// and filters out any files for so-called _other platforms_.
//
func MatchFiles(glob string) ([]string, bool) {
	filenames, err := filepath.Glob(glob)
	FatalIf(err)
	if len(filenames) == 0 {
		return nil, false
	}
	names := make([]string, 0, len(filenames))
	for _, name := range filenames {
		if otherPlatforms.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	return names, len(names) > 0
}

// FileDefinesMain determines if a file defines a main() function
// indicating the file represents a _program_ rather than just
// being part of a _library_.
//
func FileDefinesMain(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		log.Print(err)
		return false
	}
	defer file.Close()
	for scanner := bufio.NewScanner(file); scanner.Scan(); {
		line := scanner.Text()
		if mainregex.MatchString(line) {
			return true
		}
	}
	return false
}

// MakeFilename creates a filename from a prefix part, a stem and suffix.
//
func MakeFilename(prefix, stem, suffix string) string {
	dir, name := filepath.Dir(stem), filepath.Base(stem)
	if prefix != "" && !strings.HasPrefix(name, prefix) {
		name = prefix + name
	}
	if suffix != "" && !strings.HasSuffix(name, suffix) {
		name += suffix
	}
	return filepath.Clean(filepath.Join(dir, name))
}

// LibFilename returns the name of a static library file with the given stem.
//
func LibFilename(stem string) (name string) {
	return MakeFilename(ext.libprefix, stem, ext.libsuffix)
}

// DllFilename returns the name of a dynamic library file with the given stem.
//
func DllFilename(stem string) (name string) {
	return MakeFilename(ext.libprefix, stem, ext.dllsuffix)
}

// ExeFilename returns the name of an executable file with the given stem.
// This exists to append a ".exe" on Windows.
//
func ExeFilename(stem string) (name string) {
	name = MakeFilename("", stem, ext.exesuffix)
	return
}

// ObjectFile returns the name of an object file given the
// name of a source file.
//
func ObjectFile(path string) string {
	path = filepath.Clean(filepath.Join(filepath.Join(filepath.Dir(path), objsdir), filepath.Base(path)))
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext.objsuffix
}

// DepsFile returns the name of the dcc dependency file
// for an object file. This is used when cleaning to
// remove dcc-generated files.
//
func DepsFile(path string) string {
	dir, base := filepath.Dir(path), filepath.Base(path)
	if strings.HasSuffix(dir, objsdir) {
		return filepath.Join(dir, base)
	} else {
	    return filepath.Join(dir, depsdir, base)
	}
}

// ReadSrcs reads a "SRCS" file - a list of glob patterns
// defining the names of the source files to be compiled.
//
func ReadSrcs(r io.Reader) ([]string, error) {
	var names []string
	var err error
	for input := bufio.NewScanner(r); input.Scan(); {
		line := strings.TrimSpace(input.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		names, err = Expand(line, names)
		if err != nil {
			return nil, err
		}
	}
	return names, nil
}

func Expand(patterns string, names []string) ([]string, error) {
	for _, pattern := range strings.Fields(patterns) {
		if paths, err := filepath.Glob(pattern); err != nil {
			return nil, err
		} else {
			for _, name := range paths {
				if !otherPlatforms.MatchString(name) {
					names = append(names, name)
				}
			}
		}
	}
	return names, nil
}

// FatalIf reports a possible fatal error and exits the program
// if the error is non-nil.
//
func FatalIf(err error) {
	if err != nil {
		Fatal(err)
	}
}

// Fatal reports a fatal error and exits the program.
//
func Fatal(err error) {
	log.Fatal(err)
}

func SystemName() string {
	if os := runtime.GOOS; os != "darwin" {
		return os
	}
	return "macos"
}

// ReadDmakefile reads a 'dmakefile' from the given io.Reader
// and returns a Vars containing the variables defined by the
// file. Variables are of the form <name> = <value> where
// names are space separated tokens. Values may refer to
// previously defined values via '$' prefixed names.
// Blank lines and those beginning with a '#' are ignored.
//
func ReadDmakefile(r io.Reader, path string) (Vars, error) {
	v := NewVars()
	v["OS"] = SystemName()
	v["ARCH"] = runtime.GOARCH
	lineno := 0
	fail := func(message string) (Vars, error) {
		return nil, fmt.Errorf("%s:%d - %s", path, lineno, message)
	}
	for input := bufio.NewScanner(r); input.Scan(); {
		lineno++
		line := strings.TrimSpace(input.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		index := strings.Index(line, "=")
		if index == -1 {
			return fail("malformed line, no '='")
		}
		if index == 0 {
			return fail("malformed line, no variable name before '='")
		}
		key := strings.TrimSpace(line[0 : index-1])
		if len(strings.Fields(key)) != 1 {
			return fail("malformed line, spaces in key")
		}
		val := strings.TrimSpace(line[index+1:])
		val = v.Expand(val)
		v[key] = val
	}

	return v, nil
}

// NewVars returns a new Vars
//
func NewVars() Vars {
	return make(Vars)
}

// Expand expands any '$' prefixed variable references in the given string.
//
func (v *Vars) Expand(s string) string {
	r := strings.Fields(s)
	for index, word := range r {
		if word[0] == '$' {
			key := word[1:]
			if val, found := (*v)[key]; found {
				r[index] = val
			}
		}
	}
	return strings.Join(r, " ")
}

// IsSourceCodeDirectoy determines if a base pathname, the
// name of a directory, represents the name of a typical
// source code sub-directory used in project hierachies.
//
func IsSourceCodeDirectory(dir string) bool {
	word := strings.ToLower(dir)
	if word == "src" {
		return true
	}
	if word == "source" {
		return true
	}
	return false
}

// Getenv retrieves the value of an environment variable
// or, if it is not set, returns a default value.
//
func Getenv(name, def string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return def
}

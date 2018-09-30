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
	dot_dmake_file_filename = ".dmake"
	default_dep_file_dir    = ".dcc.d"
	default_obj_file_dir    = ".dmake.o"
)

var (
	srcs            []string // names of the source files
	kind            = ""     // dcc option "--dll" | "--exe" | "--lib"
	output_filename = ""     // output filename
	cleaning        = false  // true if cleaning
	installing      = false  // true if installing

	Cflag   = flag.String("C", "", "Change directory to `directory` before doing anything.")
	oflag   = flag.String("o", "", "Define output `filename`.")
	vflag   = flag.Bool("v", false, "Issue messages.")
	kflag   = flag.Bool("k", false, "Keep going. Don't stop on error.")
	dllflag = flag.Bool("dll", false, "Automatic dynamic, not static, libraries.")
	prefix  = flag.String("prefix", get_env_var("PREFIX", "/usr/local"), "Installation `path` prefix")
	debug   = flag.Bool("zzz", false, "Enable debugging output and dcc's --debug switch.")

	depsdir = get_env_var("DCCDEPS", default_dep_file_dir)
	objsdir = get_env_var("OBJDIR", default_obj_file_dir)

	// Matches platforms other than this one. Used to filter *out*
	// Go-style platform-specific filenames from the contents of a
	// directory.
	//
	other_platform_names *regexp.Regexp

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
	main_function_regex = regexp.MustCompile("^[ \t]*(int)?[ \t]*main\\([^;]*")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("dmake: ")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: dmake [options] {exe|lib|dll} [install|clean]")
		fmt.Fprintln(os.Stderr, "       dmake [options] path...")
		fmt.Fprintln(os.Stderr, `
The first form builds, installs or cleans the specified module type located
in the current directory. Building and cleaning do the obvious.

Installing runs the "/usr/bin/install" program to copy the build output
to the appropriate directory under some "prefix" directory defined by
the -prefix option. The default is "/usr/local" so, by default, executables
install under /usr/local/bin and libraries under /usr/local/lib.

The second form runs dmake in each of the named directories in sequence,
in this mode the module type cannot be defined on the command line. Yes,
its a hack.`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *Cflag != "" {
		err := os.Chdir(*Cflag)
		maybe_fatal(err)
	}

	cwd, err := os.Getwd()
	maybe_fatal(err)

	output_filename = filepath.Base(cwd)
	if is_common_source_code_subdirectory(output_filename) {
		output_filename = filepath.Base(filepath.Dir(cwd))
	}

	if *oflag == "" {
		*oflag = output_filename
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
		checkinstall := func() bool {
			if narg < 2 {
				return false
			} else if narg > 2 || flag.Arg(1) != "install" {
				usage()
			}
			return true
		}
		switch flag.Arg(0) {
		case "install":
			installing = true
			if narg > 1 {
				usage()
			}
		case "clean":
			cleaning = true
			if narg > 1 {
				usage()
			}
		case "dll":
			kind, output_filename = "--dll", form_dll_filename(*oflag)
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		case "exe":
			kind, output_filename = "--exe", form_exe_filename(*oflag)
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		case "lib":
			kind, output_filename = "--lib", form_lib_filename(*oflag)
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		default:
			dirs = append(dirs, flag.Args()...)
		}
	}

	if len(dirs) == 0 {
		err := do_dmake(*oflag)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, dir := range dirs {
		if err := do_dmake_in(dir); err != nil {
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
	other_platform_names = regexp.MustCompile("_(" + strings.Join(names, "|") + ")\\.")
}

// do_dmake_in runs dmake in the named directory.
//
func do_dmake_in(dir string) (err error) {
	oldcwd, err := os.Getwd()
	if err != nil {
		return more_detailed_error(err, "os.Getwd")
	}
	err = os.Chdir(dir)
	if err != nil {
		return more_detailed_error(err, "os.Chdir %q", dir)
	}
	if *vflag {
		log.Println("entering directory", dir)
	}
	cwd, err2 := os.Getwd()
	if err2 != nil {
		return more_detailed_error(err2, "os.Getwd")
	}
	kind = ""
	output_filename = filepath.Base(cwd)
	err = do_dmake(filepath.Base(cwd))
	if *vflag {
		log.Println("leaving directory", dir)
	}
	err2 = os.Chdir(oldcwd)
	if err == nil {
		err = more_detailed_error(err2, "os.Chdir %q", oldcwd)
	}
	return
}

func do_dmake(opath string) (err error) {
	havefiles := false
	if dmakefile, err := os.Open(dot_dmake_file_filename); err == nil {
		err = get_vars_from_dmake_file(dmakefile, dot_dmake_file_filename)
		dmakefile.Close()
		if err != nil {
			return err
		}
		havefiles = len(srcs) > 0
	}
	if !havefiles {
		source_file_patterns := []string{"*.c", "*.cpp", "*.cc", "*.m", "*.mm"}
		for _, pattern := range source_file_patterns {
			if srcs, havefiles = glob(pattern); havefiles {
				break
			}
		}
	}
	if !havefiles {
		return fmt.Errorf("no C, Objective-C++, Objective-C or C++ source files found")
	}
	if *debug {
		log.Printf("DEBUG: srcs=%v", srcs)
	}

	// No module type defined, determine executable or library, look for main().
	//
	if kind == "" {
		for _, path := range srcs {
			if main_defined_in_source_file(path) {
				kind, output_filename = "--exe", form_exe_filename(opath)
				break
			}
		}
		if kind == "" {
			if *dllflag {
				kind, output_filename = "--dll", form_dll_filename(opath)
			} else {
				kind, output_filename = "--lib", form_lib_filename(opath)
			}
		}
		if *debug {
			log.Printf("DEBUG: inferred module type %q with name %q", kind, output_filename)
		}
	}

	if cleaning {
		os.Remove(output_filename)
		for _, path := range srcs {
			clean := func(path string, deletable string) {
				os.Remove(path)
				dir := filepath.Dir(path)
				if filepath.Base(dir) == deletable {
					os.RemoveAll(dir)
				}
			}
			ofile := object_file_filename(path)
			clean(ofile, objsdir)
			clean(dependency_file(ofile), depsdir)
		}
		return nil
	}

	args := make([]string, 0, 5+len(srcs))
	if *debug {
		args = append(args, "--debug")
	}
	args = append(args, kind, output_filename)
	args = append(args, "--objdir", objsdir)
	args = append(args, srcs...)
	cmd := exec.Command("dcc", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	os.MkdirAll(objsdir, 0777)
	if *debug {
		log.Printf("EXEC: dcc %v", args)
	}
	err = cmd.Run()

	if err == nil && installing {
		dir := get_install_dir(kind, *prefix)
		mode := "0444"
		if kind == "--exe" {
			mode = "0555"
		}
		args = []string{"-c", "-m", mode, output_filename, filepath.Join(dir, output_filename)}
		cmd = exec.Command("/usr/bin/install", args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
		if *debug {
			log.Printf("EXEC: /usr/bin/install %v", args)
		}
		err = cmd.Run()
	}

	return
}

func get_install_dir(kind, path string) string {
	if kind == "--exe" {
		return filepath.Join(path, "bin")
	}
	return filepath.Join(path, "lib")
}

// get_vars_from_dmake_file reads the dmakefile and looks for the
// standard variables:
//
//	SRCS	glob pattern matching source files
//	DLL	output a dynamic lib with the defined name
//	LIB	output a static lib with the defined name
//	EXE	output an executable with the defined name
//
func get_vars_from_dmake_file(dmakefile *os.File, path string) error {
	vars, err := read_dmake_file(dmakefile, path)
	if err != nil {
		return err
	}
	var patterns string
	var found bool
	patterns, found = vars["SRCS"]
	if found {
		srcs, err = expand_glob_patterns(patterns, srcs)
		if err != nil {
			return err
		}
		if len(srcs) < 1 {
			return fmt.Errorf("SRCS=%s matches no source files", patterns)
		}
	}

	check_var := func(varname, kindstr string, fn func(string) string) error {
		if name, exists := vars[varname]; exists {
			if kind != "" && kind != kindstr {
				return fmt.Errorf("%s definition conflicts with %s", varname, kind)
			}
			kind, output_filename = kindstr, fn(name)
		}
		return nil
	}
	if err = check_var("DLL", "--dll", form_dll_filename); err != nil {
		return err
	}
	if err = check_var("EXE", "--exe", form_exe_filename); err != nil {
		return err
	}
	if err = check_var("LIB", "--lib", form_lib_filename); err != nil {
		return err
	}
	return nil
}

// glob expands a glob-pattern to locate source files
// and filters out any files for so-called _other platforms_.
//
func glob(glob string) ([]string, bool) {
	filenames, err := filepath.Glob(glob)
	maybe_fatal(err)
	if len(filenames) == 0 {
		return nil, false
	}
	names := make([]string, 0, len(filenames))
	for _, name := range filenames {
		if other_platform_names.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	return names, len(names) > 0
}

// main_defined_in_source_file determines if a file defines a main() function
// indicating the file represents a _program_ rather than just
// being part of a _library_.
//
func main_defined_in_source_file(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		log.Print(err)
		return false
	}
	defer file.Close()
	for scanner := bufio.NewScanner(file); scanner.Scan(); {
		line := scanner.Text()
		if main_function_regex.MatchString(line) {
			return true
		}
	}
	return false
}

// form_filename creates a filename from a prefix part, a stem and suffix.
//
func form_filename(prefix, stem, suffix string) string {
	dir, name := filepath.Dir(stem), filepath.Base(stem)
	if prefix != "" && !strings.HasPrefix(name, prefix) {
		name = prefix + name
	}
	if suffix != "" && !strings.HasSuffix(name, suffix) {
		name += suffix
	}
	return filepath.Clean(filepath.Join(dir, name))
}

// form_lib_filename returns the name of a static library file with the given stem.
//
func form_lib_filename(stem string) (name string) {
	return form_filename(ext.libprefix, stem, ext.libsuffix)
}

// form_dll_filename returns the name of a dynamic library file with the given stem.
//
func form_dll_filename(stem string) (name string) {
	return form_filename(ext.libprefix, stem, ext.dllsuffix)
}

// form_exe_filename returns the name of an executable file with the given stem.
// This exists to append a ".exe" on Windows.
//
func form_exe_filename(stem string) (name string) {
	name = form_filename("", stem, ext.exesuffix)
	return
}

// object_file_filename returns the name of an object file given the
// name of a source file.
//
func object_file_filename(path string) string {
	path = filepath.Clean(filepath.Join(filepath.Join(filepath.Dir(path), objsdir), filepath.Base(path)))
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext.objsuffix
}

// dependency_file returns the name of the dcc dependency file
// for an object file. This is used when cleaning to
// remove dcc-generated files.
//
func dependency_file(path string) string {
	dir, base := filepath.Dir(path), filepath.Base(path)
	if strings.HasSuffix(dir, objsdir) {
		return filepath.Join(dir, base)
	} else {
		return filepath.Join(dir, depsdir, base)
	}
}

func more_detailed_error(err error, format string, args ...interface{}) error {
	return fmt.Errorf("%s (%s)", err, fmt.Sprintf(format, args...))
}

func expand_glob_patterns(patterns string, names []string) ([]string, error) {
	for _, pattern := range strings.Fields(patterns) {
		if paths, err := filepath.Glob(pattern); err != nil {
			return nil, more_detailed_error(err, "filepath.Glob %q", pattern)
		} else {
			for _, name := range paths {
				if !other_platform_names.MatchString(name) {
					names = append(names, name)
				}
			}
		}
	}
	return names, nil
}

func maybe_fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func get_system_name() string {
	if os := runtime.GOOS; os != "darwin" {
		return os
	}
	return "macos"
}

// read_dmake_file reads a 'dmakefile' from the given io.Reader
// and returns a Vars containing the variables defined by the
// file. Variables are of the form <name> = <value> where
// names are space separated tokens. Values may refer to
// previously defined values via '$' prefixed names.
// Blank lines and those beginning with a '#' are ignored.
//
func read_dmake_file(r io.Reader, path string) (Vars, error) {
	v := make(Vars)
	v["OS"] = get_system_name()
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
		val = interpolate_var_references(val, &v)
		v[key] = val
	}

	return v, nil
}

// Expand expands any '$' prefixed variable references in the given string.
//
func interpolate_var_references(s string, v *Vars) string {
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

// is_common_source_code_subdirectoy determines if a base pathname, the name of
// a directory, represents the name of a typical source code
// sub-directory used in project hierachies.
//
func is_common_source_code_subdirectory(dir string) bool {
	word := strings.ToLower(dir)
	if word == "src" {
		return true
	}
	if word == "source" {
		return true
	}
	return false
}

// get_env_var returns the value of an environment variable or, if it
// is not set, a default value.
//
func get_env_var(name, def string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return def
}

// dmake - build tool using dcc
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

var (
	depsdir        = getenv("DCCDEPS", ".dcc.d")
	kind           = ""
	output         = ""
	srcs           []string
	cleaning       = false
	debug          = flag.Bool("debug", false, "Enable debug output.")
	Cflag          = flag.String("C", "", "Change directory to `directory` before doing anything.")
	oflag          = flag.String("o", "", "Define output `filename`.")
	qflag          = flag.Bool("q", false, "Don't issue messages.")
	kflag          = flag.Bool("k", false, "Keep going. Don't stop on error.")
	otherPlatforms *regexp.Regexp

	// Really should make this stricter, e.g. match any of the following
	// and their forms where the return type is on a separate line and
	// allow for arbitrary whitespace.
	//
	//	int main()
	//	int main(void)
	//	int main(int
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

The second form runs dmake in each of the named directories in sequence.
The module type cannot be defined for this mode.
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *Cflag != "" {
		err := os.Chdir(*Cflag)
		Check(err)
	}

	cwd, err := os.Getwd()
	Check(err)

	output = filepath.Base(cwd)
	if output == "src" {
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

	InitOtherPlatforms()

	if len(dirs) == 0 {
		err := RunDmake()
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

func init() {
	if runtime.GOOS == "windows" {
		ext = &win
	} else if runtime.GOOS == "darwin" {
		ext = &mac
	} else { // reasonable assumption
		ext = &elf
	}
}

type Vars map[string]string

func InitOtherPlatforms() {
	platforms := []string{
		"darwin",
		"freebsd",
		"linux",
		"nacl",
		"netbsd",
		"openbsd",
		"solaris",
		"windows",
	}
	var names []string
	for _, name := range platforms {
		if name != runtime.GOOS {
			names = append(names, name)
		}
	}
	otherPlatforms = regexp.MustCompile("_(" + strings.Join(names, "|") + ")\\.")
}

func RunDmakeIn(dir string) (err error) {
	oldcwd, err := os.Getwd()
	if err != nil {
		return
	}
	err = os.Chdir(dir)
	if err != nil {
		return
	}
	if !*qflag {
		log.Println("entering directory", dir)
	}
	cwd, err2 := os.Getwd()
	if err2 != nil {
		return err2
	}
	kind = ""
	output = filepath.Base(cwd)
	*oflag = output
	err = RunDmake()
	if !*qflag {
		log.Println("leaving directory", dir)
	}
	err2 = os.Chdir(oldcwd)
	if err == nil {
		err = err2
	}
	return
}

func RunDmake() (err error) {
	var havefiles bool

	if dmakefile, err := os.Open("DMAKE"); err == nil {
		err = LoadDmakefile(dmakefile)
		dmakefile.Close()
		if err != nil {
			return err
		}
		havefiles = len(srcs) > 0
	}

	if !havefiles {
		var srcsfile *os.File
		if srcsfile, err = os.Open("SRCS"); err == nil {
			srcs, err = ReadSrcs(srcsfile)
			if len(srcs) < 1 {
				return fmt.Errorf("SRCS: no source files defined by file")
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
		srcs, havefiles = FindFiles("*.cpp")
	}
	if !havefiles {
		srcs, havefiles = FindFiles("*.cc")
	}
	if !havefiles {
		srcs, havefiles = FindFiles("*.c")
	}
	if !havefiles {
		return fmt.Errorf("no C/C++ source files found")
	}
	if *debug {
		log.Printf("FILES %v", srcs)
	}

	// No module type defined, determine executable or library, look for main().
	//
	if kind == "" {
		for _, path := range srcs {
			if FileDefinesMain(path) {
				kind, output = "--exe", ExeFilename(*oflag)
				break
			}
		}
		if kind == "" {
			kind, output = "--lib", LibFilename(*oflag)
		}
		if *debug {
			log.Printf("inferred module type %q name %q", kind, output)
		}
	}

	if cleaning {
		os.Remove(output)
		for _, path := range srcs {
			ofile := ObjectFile(path)
			dfile := DepsFile(ofile)
			os.Remove(ofile)
			os.Remove(dfile)
			// will fail if not empty but we don't care
			os.RemoveAll(filepath.Dir(dfile))
		}
		return nil
	}

	args := make([]string, 0, 3+len(srcs))
	if *debug {
		args = append(args, "--debug")
	}
	args = append(args, kind, output)
	args = append(args, srcs...)
	cmd := exec.Command("dcc", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	err = cmd.Run()
	return
}

func LoadDmakefile(dmakefile *os.File) error {
	vars, err := ReadDmakefile(dmakefile)
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

// Determine source files.
//
func FindFiles(glob string) ([]string, bool) {
	filenames, err := filepath.Glob(glob)
	Check(err)
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

// Determine if a file defines a main() function.
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

// Create a filename from a prefix part, a stem and suffix.
//
func MakeFilename(prefix, stem, suffix string) string {
	d := filepath.Dir(stem)
	b := filepath.Base(stem)
	if prefix != "" && !strings.HasPrefix(b, prefix) {
		b = prefix + b
	}
	if suffix != "" && !strings.HasSuffix(b, suffix) {
		b += suffix
	}
	return filepath.Clean(filepath.Join(d, b))
}

// Return the name of a static library file with the given stem.
//
func LibFilename(stem string) (name string) {
	return MakeFilename(ext.libprefix, stem, ext.libsuffix)
}

// Return the name of a dynamic library file with the given stem.
//
func DllFilename(stem string) (name string) {
	return MakeFilename(ext.libprefix, stem, ext.dllsuffix)
}

// Return the name of an executable file with the given stem.
//
func ExeFilename(stem string) (name string) {
	name = MakeFilename("", stem, ext.exesuffix)
	return
}

// Return the name of an object file for a source file with the given name.
//
func ObjectFile(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext.objsuffix
}

func DepsFile(path string) string {
	dir, base := filepath.Dir(path), filepath.Base(path)
	return filepath.Join(dir, depsdir, base)
}

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

func Check(err error) {
	if err != nil {
		Fatal(err)
	}
}

func Fatal(err error) {
	log.Fatal(err)
}

func ReadDmakefile(r io.Reader) (Vars, error) {
	v := NewVars()
	if os := runtime.GOOS; os != "darwin" {
		v["OS"] = os
	} else {
		v["OS"] = "macos"
	}
	v["ARCH"] = runtime.GOARCH
	for input := bufio.NewScanner(r); input.Scan(); {
		line := strings.TrimSpace(input.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		index := strings.Index(line, "=")
		if index == -1 || index == 0 {
			return nil, fmt.Errorf("malformed line")
		}
		key := strings.TrimSpace(line[0 : index-1])
		if len(strings.Fields(key)) != 1 {
			return nil, fmt.Errorf("malformed line, spaces in key")
		}
		val := strings.TrimSpace(line[index+1:])
		val = v.Expand(val)
		v[key] = val
	}

	return v, nil
}

func NewVars() Vars {
	return make(Vars)
}

func (v *Vars) Expand(s string) string {
	r := strings.Fields(s)
	for index, word := range r {
		if word[0] == '$' {
			if val, found := (*v)[word[1:]]; found {
				r[index] = val
			}
		}
	}
	return strings.Join(r, " ")
}

func getenv(name, def string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return def
}

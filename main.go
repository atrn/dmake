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
	dccCommand         = "dcc"
	dmakefileFilename  = ".dmake"
	defaultDepsFileDir = ".dcc.d"
	defaultObjFileDir  = ".objs"
	dllFileType        = "--dll"
	exeFileType        = "--exe"
	libFileType        = "--lib"

	// init options
	defaultBuildMode    = "debug"
	defaultCStandard    = "c11"
	defaultCxxStandard  = "c++14"
	defaultReleaseOptim = "-O2"
	defaultDebugOptim   = "-O0"
	defaultWarningOpts  = "-Wall -Wextra -pedantic"
)

// The state used in dmake. Collecting this until I can be bothered
// to re-factor things as methods over this and remove the globals.
// This will make sub-directory dmakes simpler.
//
// type state struct {
//	sourceFileFilenames []string
//	subdirectoryNames   []string
//	outputFileType      string
//	outputFilename      string
//	installationPrefix  string
//	dependencyFileDir   string
//	objectFileDir       string
//	buildDlls           bool
// }

var (
	sourceFileFilenames   []string // names of the source files
	subdirectoryNames     []string // names of any sub-directories
	outputFileType        = ""     // a dcc option, "--dll" | "--exe" | "--lib"
	outputFilename        = ""     // output filename
	defaultOutputFilename = ""     // default output filename
	installationPrefix    = ""

	oflag     = flag.String("o", "", "Define output `filename`.")
	vflag     = flag.Bool("v", false, "Issue messages.")
	kflag     = flag.Bool("k", false, "Keep going. Don't stop on first error.")
	dllflag   = flag.Bool("dll", false, "Create dynamic libraries.")
	prefix    = flag.String("prefix", getEnvVar("PREFIX", ""), "Installation `path` prefix.")
	debug     = flag.Bool("debug", false, "Enable dmake and dcc debug output.")
	chdir     = flag.String("C", "", "Change to `directory` before doing anything.")
	quietflag = flag.Bool("quiet", false, "Avoid output")
	depsdir   = getEnvVar("DCCDEPS", defaultDepsFileDir)
	objsdir   = getEnvVar("OBJDIR", defaultObjFileDir)

	// The platform-specific collection of file name extensions
	// and prefixes.
	//
	platform *extensions

	// This matches platforms **other** than this one. This is
	// used to ignore files using Go-style platform-specific
	// filenames.
	//
	otherPlatformNamesRegexp *regexp.Regexp

	// This matches a definition of a main() function in C/C++. This
	// could be stricter, e.g. more precisely match any of the following
	// and their forms where the return type is on a separate line and
	// arbitrary whitespace of course...
	//
	//	int main()
	//	int main(void)
	//	int main(int
	//
	// But hey, the following is enough for me...
	//
	mainFunctionRegexp = regexp.MustCompile("^[ \t]*(int)?[ \t]*main\\([^;]*")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("dmake: ")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: dmake [options] [{exe|lib|dll} [install|clean]]")
		fmt.Fprintln(os.Stderr, "       dmake [options] path...")
		fmt.Fprintln(os.Stderr, "       dmake [options] init [<options>...]")
		fmt.Fprintln(os.Stderr, `
The first form builds, installs or cleans the specified module type located
in the current directory. Building and cleaning do the obvious things and
invoke the dcc command to perform the actual building or cleaning.

The install target runs the "/usr/bin/install" program to copy the program
or library to the appropriate installation directory under some "prefix"
directory, defined by the -prefix option. The default prefix is "/usr/local"
so, by default, executables install under /usr/local/bin and libraries go
under /usr/local/lib.

The second form runs dmake in each of the named directories. No options
may be specified so dmake's module inference is used when building.
Further control is acheived by creating .dmake files.

dmake init

The third form of running dmake initializes a project's directory, creating
dcc option files and a simple Makefile to direct everything using conventional
make targets that invoke dmake appropriately.`,
		)
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *chdir != "" {
		if err := os.Chdir(*chdir); err != nil {
			log.Fatal(err)
		}
	}

	env := prepareEnv()
	narg := flag.NArg()
	args := make([]string, 0, narg)
	for _, arg := range flag.Args() {
		eq := strings.Index(arg, "=")
		if eq < 1 { // -1 or 0
			args = append(args, arg)
		} else { // arg of form <name>=<value>
			env = append(env, arg)
		}
	}
	narg = len(args)

	installing := false
	cleaning := false
	installationPrefix = *prefix

	cwd, err := os.Getwd()
	possiblyFatalError(err)
	defaultOutputFilename = filepath.Base(cwd)
	if isCommonSourceCodeSubdirectory(defaultOutputFilename) {
		defaultOutputFilename = filepath.Base(filepath.Dir(cwd))
	}
	outputFilename = defaultOutputFilename

	if narg > 0 {
		usage := func() {
			flag.Usage()
			os.Exit(1)
		}
		checkarg := func(arg string) bool {
			if narg < 2 {
				return false
			} else if narg > 2 || args[1] != arg {
				usage()
			}
			return true
		}

		// dmake init ...
		//
		//
		if args[0] == "init" {
			initProject(args[1:], cwd, *oflag)
			os.Exit(0)
		}

		checkclean := func() bool { return checkarg("clean") }
		checkinstall := func() bool { return checkarg("install") }
		switch args[0] {
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
			if *oflag != "" {
				outputFileType, outputFilename = dllFileType, *oflag
			} else {
				outputFileType, outputFilename = dllFileType, makeDllFilename(defaultOutputFilename)
			}
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		case "exe":
			if *oflag != "" {
				outputFileType, outputFilename = exeFileType, *oflag
			} else {
				outputFileType, outputFilename = exeFileType, makeExeFilename(defaultOutputFilename)
			}
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		case "lib":
			if *oflag != "" {
				outputFileType, outputFilename = libFileType, *oflag
			} else {
				outputFileType, outputFilename = libFileType, makeLibFilename(defaultOutputFilename)
			}
			if cleaning = checkclean(); !cleaning {
				installing = checkinstall()
			}
		default:
			subdirectoryNames = append(subdirectoryNames, args...)
		}
	}

	if len(subdirectoryNames) == 0 {
		opath := *oflag
		if opath == "" {
			opath = outputFilename
		}
		err := dmake(opath, cleaning, installing, env)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, dir := range subdirectoryNames {
		if err := dmakeInDirectory(dir, cleaning, installing, *vflag, env); err != nil {
			log.Println(err)
			if !*kflag {
				os.Exit(1)
			}
		}
	}
}

type Vars map[string]string

func (v *Vars) interpolateVarReferences(s string) string {
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

func dmakeInDirectory(dir string, cleaning bool, installing bool, verbose bool, env []string) (err error) {
	oldcwd, err := os.Getwd()
	if err != nil {
		return moreDetailedError(err, "os.Getwd")
	}
	err = os.Chdir(dir)
	if err != nil {
		return moreDetailedError(err, "os.Chdir %q", dir)
	}
	if verbose {
		log.Println("entering directory", dir)
	}
	cwd, err2 := os.Getwd()
	if err2 != nil {
		return moreDetailedError(err2, "os.Getwd")
	}
	outputFileType = ""
	outputFilename = filepath.Base(cwd)
	sourceFileFilenames = make([]string, 0)
	err = dmake(filepath.Base(cwd), cleaning, installing, env)
	if verbose {
		log.Println("leaving directory", dir)
	}
	err2 = os.Chdir(oldcwd)
	if err == nil && err2 != nil {
		err = moreDetailedError(err2, "os.Chdir %q", oldcwd)
	}
	return
}

var sourceFilePatterns = []string{"*.cpp", "*.cc", "*.c", "*.m", "*.mm"}

func getSourceFileFilenames() (havefiles bool, lang string) {
	var pattern string
	for _, pattern = range sourceFilePatterns {
		if sourceFileFilenames, havefiles = glob(pattern); havefiles {
			break
		}
	}
	if havefiles {
		switch pattern {
		case "*.c":
			lang = "c"
		case "*.cpp", "*.cc":
			lang = "c++"
		case "*.m":
			lang = "objc"
		case "*.mm":
			lang = "objc++"
		}
	} else {
		lang = ""
	}
	return
}

func determineOutput(opath string) (outputFileType string, outputFilename string) {
	for _, path := range sourceFileFilenames {
		if sourceFileDefinesMain(path) {
			outputFileType, outputFilename = exeFileType, makeExeFilename(opath)
			break
		}
	}
	if outputFileType == "" {
		if *dllflag {
			outputFileType, outputFilename = dllFileType, makeDllFilename(opath)
		} else {
			outputFileType, outputFilename = libFileType, makeLibFilename(opath)
		}
	}
	if *debug {
		log.Printf("DEBUG: module inferred as \"%s %s\"", outputFileType, outputFilename)
	}
	return
}

func dmake(opath string, cleaning bool, installing bool, env []string) (err error) {
	havefiles := false
	if dmakefile, err := os.Open(dmakefileFilename); err == nil {
		err = getVarsFromDmakeFile(dmakefile, dmakefileFilename)
		dmakefile.Close()
		if err != nil {
			return err
		}
		havefiles = len(sourceFileFilenames) > 0
	}
	if !havefiles {
		havefiles, _ = getSourceFileFilenames()
	}
	if !havefiles {
		return fmt.Errorf("no C, Objective-C++, Objective-C or C++ source files found")
	}
	if *debug {
		log.Printf("DEBUG: sourceFileFilenames=%v", sourceFileFilenames)
	}

	// If no module type is define we have to determine if the module is an
	// executable or library. So we look for a main() function.
	//
	if outputFileType == "" {
		outputFileType, outputFilename = determineOutput(opath)
	}

	if cleaning {
		os.Remove(outputFilename)
		for _, path := range sourceFileFilenames {
			clean := func(path string, deletable string) {
				os.Remove(path)
				dir := filepath.Dir(path)
				if filepath.Base(dir) == deletable {
					os.RemoveAll(dir)
				}
			}
			ofile := makeObjectFileFilename(path)
			clean(ofile, objsdir)
			clean(objectFilesDependencyFile(ofile), depsdir)
		}
		return nil
	}

	os.MkdirAll(filepath.Dir(outputFilename), 0777)

	args := make([]string, 0, 5+len(sourceFileFilenames))
	if *debug {
		args = append(args, "--debug")
	}
	if *quietflag {
		args = append(args, "--quiet")
	}
	args = append(args, outputFileType, outputFilename)
	args = append(args, "--objdir", objsdir)
	args = append(args, sourceFileFilenames...)
	cmd := exec.Command(dccCommand, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	os.MkdirAll(objsdir, 0777)
	if *debug {
		log.Printf("ENV: %v", env)
		log.Printf("RUN: %s %v", dccCommand, args)
	}
	err = cmd.Run()

	if err == nil && installing {
		path := installationPrefix
		if path == "" {
			path = "."
		}
		dir := getInstallDir(outputFileType, path)
		mode := "0444"
		if outputFileType == exeFileType {
			mode = "0555"
		}
		args = []string{"-c", "-m", mode, outputFilename, filepath.Join(dir, outputFilename)}
		cmd = exec.Command("/usr/bin/install", args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
		cmd.Env = env
		if *debug {
			log.Printf("RUN: /usr/bin/install %v", args)
		}
		err = cmd.Run()
	}

	return
}

func getInstallDir(kind, path string) string {
	if kind == exeFileType {
		return filepath.Join(path, "bin")
	}
	return filepath.Join(path, "lib")
}

// getVarsFromDmakeFile reads the dmakefile and looks for the
// standard variables:
//
//	SRCS	glob pattern matching source files
//	DLL	output a dynamic lib with the defined name
//	LIB	output a static lib with the defined name
//	EXE	output an executable with the defined name
//	DIRS	sub-directories to be built
//	PREFIX	installation prefix
//
func getVarsFromDmakeFile(dmakefile *os.File, path string) error {
	vars, err := readDmakeFile(dmakefile, path)
	if err != nil {
		return err
	}

	var patterns string
	var found bool
	patterns, found = vars["SRCS"]
	if found {
		sourceFileFilenames, err = expandGlobPatterns(patterns, sourceFileFilenames)
		if err != nil {
			return err
		}
		if len(sourceFileFilenames) < 1 {
			return fmt.Errorf("SRCS=%s matches no source files", patterns)
		}
	}

	if path, found := vars["PREFIX"]; found {
		if installationPrefix == "" {
			installationPrefix = path
		}
	}

	var directories string
	directories, found = vars["DIRS"]
	if found {
		subdirectoryNames, err = expandGlobPatterns(directories, subdirectoryNames)
		if err != nil {
			return err
		}
		if len(subdirectoryNames) < 1 {
			return fmt.Errorf("DIRS=%s matches no names", patterns)
		}
	}

	checkVar := func(name, fileType string, fn func(string) string) error {
		if name, exists := vars[name]; exists {
			if outputFileType != "" && outputFileType != fileType {
				return fmt.Errorf("%s definition conflicts with %s", name, outputFileType)
			}
			outputFileType = fileType
			outputFilename = fn(name)
		}
		return nil
	}
	if err = checkVar("DLL", dllFileType, makeDllFilename); err != nil {
		return err
	}
	if err = checkVar("EXE", exeFileType, makeExeFilename); err != nil {
		return err
	}
	if err = checkVar("LIB", libFileType, makeLibFilename); err != nil {
		return err
	}
	return nil
}

// glob expands a glob-pattern to locate source files
// and filters out any files for so-called _other platforms_.
//
func glob(pattern string) ([]string, bool) {
	filenames, err := filepath.Glob(pattern)
	possiblyFatalError(err)
	if len(filenames) == 0 {
		return nil, false
	}
	names := make([]string, 0, len(filenames))
	for _, name := range filenames {
		if otherPlatformNamesRegexp.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	return names, len(names) > 0
}

// sourceFileDefinesMain determines if a file defines a main() function
// indicating the file represents a _program_ rather than just
// being part of a _library_.
//
func sourceFileDefinesMain(path string) bool {
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

func makeFilenameFrom(path, prefix, suffix string) string {
	dirname, basename := filepath.Dir(path), filepath.Base(path)
	if prefix != "" && !strings.HasPrefix(basename, prefix) {
		basename = prefix + basename
	}
	if suffix != "" && !strings.HasSuffix(basename, suffix) {
		basename += suffix
	}
	return filepath.Clean(filepath.Join(dirname, basename))
}

func makeLibFilename(stem string) (name string) {
	return makeFilenameFrom(stem, platform.libprefix, platform.libsuffix)
}

func makeDllFilename(stem string) (name string) {
	return makeFilenameFrom(stem, platform.dllprefix, platform.dllsuffix)
}

func makeExeFilename(stem string) (name string) {
	name = makeFilenameFrom(stem, "", platform.exesuffix)
	return
}

func makeObjectFileFilename(path string) string {
	dirname, basename := filepath.Dir(path), filepath.Base(path)
	path = filepath.Clean(filepath.Join(filepath.Join(dirname, objsdir), basename))
	return strings.TrimSuffix(path, filepath.Ext(basename)) + platform.objsuffix
}

func objectFilesDependencyFile(path string) string {
	dirname, basename := filepath.Dir(path), filepath.Base(path)
	if strings.HasSuffix(dirname, objsdir) {
		return filepath.Join(dirname, basename)
	} else {
		return filepath.Join(dirname, depsdir, basename)
	}
}

func moreDetailedError(err error, format string, args ...interface{}) error {
	return fmt.Errorf("%s (%s)", err, fmt.Sprintf(format, args...))
}

func expandGlobPatterns(patterns string, names []string) ([]string, error) {
	for _, pattern := range strings.Fields(patterns) {
		if paths, err := filepath.Glob(pattern); err != nil {
			return nil, moreDetailedError(err, "filepath.Glob %q", pattern)
		} else {
			for _, name := range paths {
				if !otherPlatformNamesRegexp.MatchString(name) {
					names = append(names, name)
				}
			}
		}
	}
	return names, nil
}

func possiblyFatalError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getSystemName() string {
	return runtime.GOOS
}

// Read a 'dmakefile' from the given io.Reader and return a Vars
// containing the variables it defines. Variables are of the form
// <name> = <value>, names is a single, space separated, token. Values
// may refer to previously defined values via '$' prefixed names.
// Blank lines and those beginning with '#' are ignored.
//
func readDmakeFile(r io.Reader, path string) (Vars, error) {
	vars := make(Vars)
	vars["OS"] = getSystemName()
	vars["ARCH"] = runtime.GOARCH
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
		key := strings.TrimSpace(line[0:index])
		if len(strings.Fields(key)) != 1 {
			return fail("malformed line, spaces in key")
		}
		val := strings.TrimSpace(line[index+1:])
		val = vars.interpolateVarReferences(val)
		vars[key] = val
	}

	return vars, nil
}

func isCommonSourceCodeSubdirectory(dir string) bool {
	word := strings.ToLower(dir)
	if word == "src" {
		return true
	}
	if word == "source" {
		return true
	}
	return false
}

func getEnvVar(name, def string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return def
}

func prepareEnv() []string {
	e := make([]string, 0)

	goodVars := []string{
		// Standard/common names
		"HOME",
		"LOGNAME",
		"PATH",
		"SHELL",
		"TERM",
		"TERMCAP",
		"USER",
		// dcc recognized
		"CC",
		"CXX",
		"NJOBS",
		"CCFILE",
		"CXXFILE",
		"CFLAGSFILE",
		"CXXFLAGSFILE",
		"LDFLAGSFILE",
		"LIBSFILE",
	}
	for _, name := range goodVars {
		if value := os.Getenv(name); value != "" {
			e = append(e, fmt.Sprintf("%s=%s", name, value))
		}
	}
	return e
}

// platform-specific file naming stuff
//

type extensions struct {
	objsuffix string
	exesuffix string
	libprefix string
	libsuffix string
	dllprefix string
	dllsuffix string
}

func init() {
	var (
		win = extensions{".obj", ".exe", "", ".lib", "", ".dll"}
		mac = extensions{".o", "", "lib", ".a", "lib", ".dylib"}
		elf = extensions{".o", "", "lib", ".a", "lib", ".so"}
	)
	name := getSystemName()
	switch name {
	case "windows":
		platform = &win
	case "darwin":
		platform = &mac
	default:
		platform = &elf
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
	otherPlatformNamesRegexp = regexp.MustCompile("_(" + strings.Join(names, "|") + ")\\.")
}

// dmake init [<name> <options>...]
//
// options :=
//            exe | lib | dll
// 	    | c | c++ | objc | objc++
//          | c99 | c11
//          | c++11 | c++14 | c++17 | c++20
//          | debug | release
//
// Creates:
//
//	.dcc/CXXFLAGS (or .dcc/CFLAGS for C)
//	.dcc/LDFLAGS (only if required)
//	.dcc/LIBS (only if required)
//	.dmake (only if required)
//	Makefile
//
func initProject(args []string, cwd string, opath string) {

	//  Don't do anything if there is already something called .dcc
	//
	if _, err := os.Stat(".dcc"); err == nil {
		log.Fatal("a .dcc directory already exists, not continuing")
	}
	//  Don't do anything if there is already something called .dmake
	//
	if _, err := os.Stat(".dmake"); err == nil {
		log.Fatal("a .dmake file already exists, not continuing")
	}
	//  Don't do anything if there is already something called Makefile
	//
	if _, err := os.Stat("Makefile"); err == nil {
		log.Fatal("a Makefile already exists, not continuing")
	}

	var (
		projectType string
		outputName  string
		language    string
		languageStd string
		buildMode   string
	)

	tag := func(w io.Writer) {
		fmt.Fprintln(w, "# This file is read by dcc")
		fmt.Fprintln(w)
	}

	touch := func(path string) {
		file, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		tag(file)
		if err := file.Close(); err != nil {
			os.Remove(path)
			log.Fatal(err)
		}
	}

	cannotAlreadyHave := func(value, arg, what string) string {
		if value != "" {
			log.Fatal(": " + what + " already specified")
		}
		return arg
	}

	_, language = getSourceFileFilenames()

	for _, arg := range args {
		switch arg {
		case "c", "c++", "objc", "objc++":
			if language != "" {
				if language != arg {
					log.Fatal(arg + ": conflicts with source file language, " + language)
				}
			} else {
				language = arg
			}
		case "exe", "lib", "dll":
			projectType = cannotAlreadyHave(projectType, arg, "project type")
		case "debug", "release":
			buildMode = cannotAlreadyHave(buildMode, arg, "build mode")
		case "c99", "c11":
			if language == "c++" {
				log.Fatal("C standard specified but this is a C++ project")
			}
			languageStd = cannotAlreadyHave(languageStd, arg, "language standard")
		case "c++11", "c++14", "c++17", "c++20":
			if language == "c" {
				log.Fatal("C++ standard specified but this is a C++ project")
			}
			languageStd = cannotAlreadyHave(languageStd, arg, "language standard")
		default:
			outputName = cannotAlreadyHave(outputName, arg, "output filename")
		}
	}

	if outputName == "" {
		outputName = defaultOutputFilename
	}
	if buildMode == "" {
		buildMode = defaultBuildMode
	}
	if languageStd == "" {
		if language == "c" {
			languageStd = defaultCStandard
		} else if language == "c++" {
			languageStd = defaultCxxStandard
		}
	}

	if err := os.Mkdir(".dcc", 0777); err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}

	//  Create the dcc options file, CFLAGS or CXXFLAGS.
	//
	optionsFilename := ".dcc/CFLAGS"
	if language == "c++" {
		optionsFilename = ".dcc/CXXFLAGS"
	}

	file, err := os.Create(optionsFilename)
	if err != nil {
		log.Fatal(err)
	}
	tag(file)
	if languageStd != "" {
		fmt.Fprintf(file, "-std=%s\n", languageStd)
	}
	fmt.Fprintln(file, defaultWarningOpts)
	fmt.Fprintln(file, "-g")
	if buildMode == "release" {
		fmt.Fprintln(file, "-DNDEBUG")
		fmt.Fprintln(file, defaultReleaseOptim)
	} else { // if buildMode == "debug"
		fmt.Fprintln(file, "-DDEBUG")
		fmt.Fprintln(file, defaultDebugOptim)
	}

	if err := file.Close(); err != nil {
		os.Remove(optionsFilename)
		log.Fatal(err)
	}

	var typeVarName string
	switch projectType {
	case "":
		typeVarName = libFileType
	case "exe":
		projectType = exeFileType
		typeVarName = "EXE"
		touch(".dcc/LDFLAGS")
		touch(".dcc/LIBS")
	case "lib":
		projectType = libFileType
	case "dll":
		projectType = dllFileType
		typeVarName = "DLL"
		touch(".dcc/LDFLAGS")
	default:
		log.Fatal(projectType + ": unsupported project type")
	}

	//  Do we need to create a .dmake file?
	//
	//  That depends. If the output name is not the same as the
	//  default, then yes, we need to use a .dmake file in order
	//  to set that name. But to do that we need to know what type
	//  of project we're building so we can output the appropriate
	//  "<type> = <name>" setting used in the .dmake file. Now, if
	//  the user didn't tell us that we have to figure it out but
	//  in that situation we need the names of the the source
	//  files so we can scan them, looking for a definition of
	//  main() to figure out if we're building an executable or a
	//  library. Phew. But luckily we do that anyway and can
	//  re-use the determineOutput function.
	//
	if outputName != defaultOutputFilename {
		if projectType == "" {
			projectType, _ = determineOutput(opath)
		}
		file, err := os.Create(".dmake")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(file, "%s = %s\n", typeVarName, outputName)
		if err := file.Close(); err != nil {
			os.Remove(".dmake")
			log.Fatal(err)
		}
	}

	// Output the Makefile
	//
	makefile, err := os.Create("Makefile")
	if err != nil {
		log.Fatal(err)
	}

	installDir := "$(prefix/lib"
	if projectType == exeFileType {
		installDir = "$(prefix)/bin"
	}

	fmt.Fprintf(makefile, `.PHONY: all clean install
prefix?=/usr/local
quiet?=@
sudo?=
all:; $(quiet) dmake
clean:; $(quiet) dmake clean
install: all; $(quiet) $(sudo) install -c %s %s
`,
		outputName,
		installDir,
	)

	if err := makefile.Close(); err != nil {
		os.Remove("Makefile")
		log.Fatal(err)
	}
}

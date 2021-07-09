// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	dmakeFileFilename = ".dmake"

	// dcc related
	dccCommandName     = "dcc"
	defaultDepsFileDir = ".dcc.d"
	defaultObjFileDir  = ".objs"

	// dmake init defaults
	defaultBuildMode    = "debug"
	defaultCStandard    = "c11"
	defaultCxxStandard  = "c++14"
	defaultReleaseOptim = "-O2"
	defaultDebugOptim   = "-O0"
	defaultWarningOpts  = "-Wall -Wextra -pedantic"
)

type Dmake struct {
	sourceFiles         []string   // names of the source files to be compiled
	outputtype          OutputType // type of thing being built
	outputname          string     // output filename
	outputnameDefaulted bool       // true if the user did NOT define outputname
	defaultoutput       string     // default output filename
	installprefix       string     // where to install
	directories         []string   // names of any sub-directories to be compiled
}

//  Create a new Dmake
//
func NewDmake(dir string, outputName string, installPrefix string) *Dmake {
	dmake := &Dmake{installprefix: installPrefix}
	basename := filepath.Base(dir)
	if basename == "src" || basename == "source" {
		dmake.defaultoutput = filepath.Base(filepath.Dir(dir))
	} else {
		dmake.defaultoutput = basename
	}
	if outputName != "" {
		dmake.outputname = outputName
		dmake.outputnameDefaulted = false
	} else {
		dmake.outputname = dmake.defaultoutput
		dmake.outputnameDefaulted = true
	}
	return dmake
}

// Do dmake some-action in cwd
//
func (dmake *Dmake) Run(action Action, env []string) error {
	if *debug {
		log.Print("DEBUG: action=", action.String())
	}

	err := dmake.ReadDmakefile()
	if err != nil {
		return err
	}

	if dmake.HaveDirs() {
		err = dmake.Directories(action, env)
		if err != nil {
			return err
		}
	}

	if len(dmake.sourceFiles) < 1 {
		dmake.sourceFiles, _, err = SourceFiles()
		if err != nil {
			return err
		}
	}

	if len(dmake.sourceFiles) < 1 {
		if !dmake.HaveDirs() {
			return fmt.Errorf("no C, Objective-C++, Objective-C or C++ source files found")
		} else {
			return nil
		}
	}

	if *debug {
		log.Printf("DEBUG: sourceFiles=%q", dmake.sourceFiles)
	}

	if dmake.outputtype == UnknownOutputType {
		dmake.outputtype = dmake.DetermineOutputType()
		if dmake.outputnameDefaulted {
			dmake.SetOutputNameFromType()
		}
	}

	if action == Cleaning {
		return dmake.CleanAction()
	}

	err = dmake.BuildAction(env)
	if err != nil {
		return err
	}

	if action == Installing {
		err = dmake.InstallAction()
	}
	return err
}

func (dmake *Dmake) SetOutputNameFromType() {
	switch dmake.outputtype {
	case DllOutputType:
		dmake.outputname = platform.DllFilename(dmake.outputname)
	case PluginOutputType:
		dmake.outputname = platform.PluginFilename(dmake.outputname)
	case ExeOutputType:
		dmake.outputname = platform.ExeFilename(dmake.outputname)
	case LibOutputType:
		dmake.outputname = platform.LibFilename(dmake.outputname)
	default:
		panic("outputtype not set when it should be known by now")
	}
}

//  Perform some action across the defined sub-directories
//
func (dmake *Dmake) Directories(action Action, env []string) (result error) {
	if *debug {
		log.Printf("DEBUG: directories %q", dmake.directories)
	}

	for _, path := range dmake.directories {
		if *verbose {
			log.Printf("entering %q", path)
		}

		savedCwd, err := ChangeDirectory(path)
		if err != nil {
			return err
		}

		err = NewDmake(path, "", dmake.installprefix).Run(action, env)
		if err != nil {
			if !*keepgoing {
				return err
			}
			if result == nil {
				result = err
			}
		}

		if *verbose {
			log.Printf(" leaving %q", path)
		}

		savedCwd.Restore()
	}
	return
}

// Build usng dcc
//
func (dmake *Dmake) BuildAction(env []string) error {
	os.MkdirAll(filepath.Dir(dmake.outputname), 0777)
	os.MkdirAll(objsdir, 0777)

	dccArgs := make([]string, 0, 5+len(dmake.sourceFiles))
	if *dccdebug {
		dccArgs = append(dccArgs, "--debug")
	}
	if *quietflag {
		dccArgs = append(dccArgs, "--quiet")
	}
	dccArgs = append(dccArgs, dmake.outputtype.DccArgument(), dmake.outputname)
	dccArgs = append(dccArgs, "--objdir", objsdir)
	dccArgs = append(dccArgs, dmake.sourceFiles...)

	cmd := exec.Command(dccCommandName, dccArgs...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	if *debug {
		log.Printf("RUN: %s %v", dccCommandName, dccArgs)
	}
	return cmd.Run()
}

// dmake clean in cwd
//
func (dmake *Dmake) CleanAction() error {
	os.Remove(dmake.outputname)
	for _, srcfile := range dmake.sourceFiles {
		doClean := func(path string, deletable string) {
			os.Remove(path)
			dir := filepath.Dir(path)
			if filepath.Base(dir) == deletable {
				os.RemoveAll(dir)
			}
		}
		ofile := ObjectFilename(srcfile, objsdir)
		doClean(ofile, objsdir)
		doClean(DependenciesFilename(ofile, depsdir), depsdir)
	}
	return nil
}

// dmake install in cwd
//
func (dmake *Dmake) InstallAction() error {
	path := dmake.installprefix
	if path == "" {
		path = "."
	}
	var (
		dest string
		mode os.FileMode
	)
	if dmake.outputtype == ExeOutputType {
		dest = filepath.Join(path, "bin")
		mode = os.FileMode(0555)
	} else {
		dest = filepath.Join(path, "lib")
		mode = os.FileMode(0444)
	}
	return platform.installfile(dmake.outputname, filepath.Join(dest, dmake.outputname), mode)
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
func (dmake *Dmake) InitAction(args []string, cwd string) error {

	var err error

	//  Don't do anything if there is already something called .dcc
	//
	if _, err = os.Stat(".dcc"); err == nil {
		return errors.New("a .dcc directory already exists, not continuing")
	}
	//  Don't do anything if there is already something called .dmake
	//
	if _, err = os.Stat(".dmake"); err == nil {
		return errors.New("a .dmake file already exists, not continuing")
	}
	//  Don't do anything if there is already something called Makefile
	//
	if _, err = os.Stat("Makefile"); err == nil {
		return errors.New("a Makefile already exists, not continuing")
	}

	var (
		projectType string
		outputName  string
		language    Language
		languageStd string
		buildMode   string
	)

	alreadyHave := func(what, value, arg string) {
		log.Fatalf("%s: %s already specified as %s", arg, what, value)
	}

	_, language, err = SourceFiles()
	if err != nil {
		return err
	}

	for _, arg := range args {
		switch arg {
		case "c", "c++", "objc", "objc++":
			if language != UnknownLanguage && language.String() != arg {
				log.Fatal(arg + " is not the language used by source files, " + language.String())
			}
		case "exe", "lib", "dll":
			if projectType != "" {
				alreadyHave("project type", projectType, arg)
			}
			projectType = arg
		case "debug", "release":
			if buildMode != "" {
				alreadyHave("build mode", buildMode, arg)
			}
			buildMode = arg
		case "c99", "c11":
			if language == CplusplusLanguage {
				log.Fatal("C standard specified but this is a C++ project")
			}
			if languageStd != "" {
				alreadyHave("language standard", languageStd, arg)
			}
			languageStd = arg
		case "c++11", "c++14", "c++17", "c++20":
			if language == CLanguage {
				log.Fatal("C++ standard specified but this is a C++ project")
			}
			if languageStd != "" {
				alreadyHave("language standard", languageStd, arg)
			}
			languageStd = arg
		default:
			if outputName != "" {
				alreadyHave("output filename", outputName, arg)
			}
			outputName = arg
		}
	}

	if outputName == "" {
		outputName = dmake.defaultoutput
	}
	if buildMode == "" {
		buildMode = defaultBuildMode
	}
	if languageStd == "" {
		if language == CLanguage {
			languageStd = defaultCStandard
		} else if language == CplusplusLanguage {
			languageStd = defaultCxxStandard
		}
	}

	if err := os.Mkdir(".dcc", 0777); err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}

	//  Create the dcc options file, CFLAGS or CXXFLAGS.
	//
	optionsFilename := ".dcc/CFLAGS"
	if language == CplusplusLanguage {
		optionsFilename = ".dcc/CXXFLAGS"
	}

	file, err := os.Create(optionsFilename)
	if err != nil {
		log.Fatal(err)
	}
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

	const readByDccComment = "# This file is read by dcc\n#\n\n"

	var typeVarName string

	switch projectType {
	case "":
		switch dmake.DetermineOutputType() {
		case ExeOutputType:
			typeVarName = "EXE"
		case DllOutputType:
			typeVarName = "DLL"
		case PluginOutputType:
			typeVarName = "PLUGIN"
		case LibOutputType:
			typeVarName = "LIB"
		}
	case "dll":
		typeVarName = "DLL"
		CreateFile(".dcc/LDFLAGS", readByDccComment)
	case "exe":
		typeVarName = "EXE"
		CreateFile(".dcc/LDFLAGS", readByDccComment)
		CreateFile(".dcc/LIBS", readByDccComment)
	case "lib":
		typeVarName = "LIB"
	default:
		log.Fatal(projectType + ": unsupported project type")
	}

	//  Do we need to create a .dmake file?
	//
	//  If the output name is not the same as the default that dcc
	//  would use, then yes, we need to use a .dmake file to set
	//  the name.
	//
	//  However, to do that we need to know what type of project
	//  we're building so we can output the appropriate "<type> =
	//  <name>" setting used in the .dmake file.
	//
	//  If the user didn't tell us that we have to figure it out
	//  from the source files, if they exist.
	//
	if outputName != dmake.defaultoutput {
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
	if projectType == "exe" {
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

	err = makefile.Close()
	if err != nil {
		os.Remove("Makefile")
	}
	return err
}

// Determine the type of the build product
//
func (dmake *Dmake) DetermineOutputType() OutputType {
	outputtype := UnknownOutputType
	for _, path := range dmake.sourceFiles {
		if DefinesMain(path) {
			outputtype = ExeOutputType
			break
		}
	}
	if outputtype == UnknownOutputType {
		if *dllflag {
			outputtype = DllOutputType
		} else if *pluginflag {
			outputtype = PluginOutputType
		} else {
			outputtype = LibOutputType
		}
	}
	if *debug {
		log.Printf("DEBUG: module type %q", outputtype)
	}
	return outputtype
}

//  Read a .dmake file and set up the receiver from the variables
//  defined in that file.
//
func (dmake *Dmake) ReadDmakefile() (err error) {
	vars := make(Vars)
	err = vars.ReadFromFile(dmakeFileFilename)
	if err == nil {
		err = dmake.InitFromVars(vars)
	} else if os.IsNotExist(err) {
		err = nil
	}
	return
}

//  Set up the receiver from a Vars. Specifically,
//
//	SRCS	glob pattern matching source files
//	DLL	output a dynamic lib with the defined name
//	LIB	output a static lib with the defined name
//	EXE	output an executable with the defined name
//	DIRS	sub-directories to be built
//	PREFIX	installation prefix
//
func (dmake *Dmake) InitFromVars(vars Vars) error {
	var patterns string
	var found bool
	var err error

	patterns, found = vars.GetValue("SRCS")
	if found {
		dmake.sourceFiles, err = ExpandGlobs(patterns)
		if err != nil {
			return err
		}
		if len(dmake.sourceFiles) < 1 {
			return fmt.Errorf("SRCS=%s matches no source files", patterns)
		}
	}

	if path, found := vars.GetValue("PREFIX"); found {
		if dmake.installprefix == "" {
			dmake.installprefix = path
		}
	}

	var directories string
	directories, found = vars.GetValue("DIRS")
	if found {
		dmake.directories, err = ExpandGlobs(directories)
		if err != nil {
			return err
		}
		if len(dmake.directories) < 1 {
			return fmt.Errorf("DIRS=%s matches no names", patterns)
		}
	}

	checkVar := func(name string, outputtype OutputType, fn func(string) string) error {
		if name, exists := vars.GetValue(name); exists {
			if dmake.outputtype != UnknownOutputType && dmake.outputtype != outputtype {
				return fmt.Errorf("%s definition conflicts with %s", name, dmake.outputtype.String())
			}
			dmake.outputtype = outputtype
			dmake.outputname = fn(name)
		}
		return nil
	}
	if err = checkVar("DLL", DllOutputType, platform.DllFilename); err != nil {
		return err
	}
	if err = checkVar("PLUGIN", PluginOutputType, platform.PluginFilename); err != nil {
		return err
	}
	if err = checkVar("EXE", ExeOutputType, platform.ExeFilename); err != nil {
		return err
	}
	if err = checkVar("LIB", LibOutputType, platform.LibFilename); err != nil {
		return err
	}
	return nil
}

//  Add a directory to the receiver's list of directories to be dmake'd.
//
func (dmake *Dmake) AddDirectory(paths ...string) {
	dmake.directories = append(dmake.directories, paths...)
}

//  Return true if the receiver has subdirectories.
//
func (dmake *Dmake) HaveDirs() bool {
	return len(dmake.directories) > 0
}

//  Set the output type of the receiver.
//
func (dmake *Dmake) SetOutputType(outputtype OutputType) {
	dmake.outputtype = outputtype
	if dmake.outputname == "" {
		dmake.outputname = FilenameForType(outputtype, dmake.defaultoutput)
	}
}

// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	langflag Language = UnknownLanguage

	chdir     = flag.String("C", "", "Change to `directory` before doing anything.")
	dllflag   = flag.Bool("dll", false, "Implicitly create DLLs instead of static libraries.")
	keepgoing = flag.Bool("k", false, "Keep going. Don't stop on first error.")
	oflag     = flag.String("o", "", "Define output `filename`.")
	prefix    = flag.String("prefix", Getenv("PREFIX", ""), "Installation `path` prefix.")

	debug     = flag.Bool("debug", false, "Enable dmake debug output.")
	dccdebug  = flag.Bool("dcc-debug", false, "Enable dcc debug output")
	verbose   = flag.Bool("v", false, "Issue messages.")
	quietflag = flag.Bool("quiet", false, "Avoid output")

	depsdir = Getenv("DCCDEPS", defaultDepsFileDir)
	objsdir = Getenv("OBJDIR", defaultObjFileDir)
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("dmake: ")

	action := DefaultAction
	env := os.Environ()

	flag.Var(&langflag, "lang", "Assume all source files are `lang` (one of 'c', 'c++', 'objc', 'objc++')")

	flag.Usage = outputUsage
	flag.Parse()

	if *debug {
		*verbose = true
	}

	if *chdir != "" {
		if err := os.Chdir(*chdir); err != nil {
			log.Fatal(err)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Collect command line arguments and add any <name>=<value>
	// to the environment slice passed to dcc.
	//
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

	dmake := NewDmake(cwd, *oflag, *prefix)
	initArgsIndex := -1

loop:
	for argi, arg := range args {
		switch arg {
		case "init":
			if action != DefaultAction {
				flag.Usage()
				os.Exit(1)
			}
			action = Initing
			initArgsIndex = argi + 1
			break loop
		case "build":
			if action != DefaultAction {
				flag.Usage()
				os.Exit(1)
			}
			action = Building
		case "install":
			if action != DefaultAction {
				flag.Usage()
				os.Exit(1)
			}
			action = Installing
		case "clean":
			if action != DefaultAction {
				flag.Usage()
				os.Exit(1)
			}
			action = Cleaning
		case "dll":
			dmake.SetOutputType(DllOutputType)
		case "exe":
			dmake.SetOutputType(ExeOutputType)
		case "lib":
			dmake.SetOutputType(LibOutputType)
		default:
			dmake.AddDirectory(arg)
		}
	}

	if dmake.HaveDirs() && *oflag != "" {
		log.Fatal("-o flag not supported when building directories")
	}

	if action == Initing {
		err = dmake.Init(args[initArgsIndex:], cwd)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if action == DefaultAction {
		action = Building
	}

	if err = dmake.ReadDmakefile(); err != nil {
		log.Fatal(err)
	}

	err = dmake.Run(action, env)
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(0)
}

func outputUsage() {
	fmt.Fprintln(os.Stderr, "usage: dmake [options] [{exe|lib|dll} [install|clean]]")
	fmt.Fprintln(os.Stderr, "       dmake [options] path...")
	fmt.Fprintln(os.Stderr, "       dmake [options] init [<init-options>...]")
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

# dmake - build tool using dcc

`dmake` is a tool for building C++ and C programs using the `dcc`
compiler driver as the underlying build tool. Because dcc itself does
all dependency analysis dmake simply determines what is being built
and invokes dcc accordingly. Compiling and linking if required.

Despite its name dmake is *not* a make clone and has no relation to
make or tools of its ilk. The name `dmake` is used to match `dcc`.

`dmake` uses the dcc compiler-driver to build _modules_ of different
types. Modules are either programs, called _exes_, dynamic libraries
or static libraries. The different module types are distinguished by
requiring different commands to create the module's outputs and the
file names of those outputs.

`dmake` uses `dcc` to take care of all compilation and library
construction. `dmake` determines what and how to build and relies upon
dcc's automatic, dependency-based, compilations and library generation.

So `dmake` is wrapper but it offers some features to the lazy
programmer to reduce their workload.  If not told otherwise dmake will
automatically determine the name and type of thing being built,
whether it is a program or a library, and then invokes dcc with
appropriate options to create the program or library. The program
doesn't need to do anything.

Build options are defined using dcc _option files_, text files that
list build options for the different build steps (these files are
named using the conventional make-macro name of the corresponding set
of options, e.g C compiler options are read from the _CFLAGS_ file,
C++ compiler compiler options from _CXXFLAGS_ and so on).

The dmake _user-experience_ is typically a two step thing. The first
step is where we create the _options files_ used by dcc to define the
compiler and linker options. We can create a .dcc/CXXFLAGS file for
compiler options, .dcc/LDFLAGS for linker options. If we're building a
program we may create another file, .dcc/LIBS, to define any required
libraries.

Once the options files are in place usage is usually just typing the
single word command `dmake`. It figures out what to do. If it can't
you can tell it with commands like `dmake exe` and `dmake lib clean`.

## Using dmake

dmake's actions depends upon how it in invoked. It can either infer
the module type, executable or library, or can be told the type.

When invoked without arguments dmake attempts to infer the module type
by reading the contents of the source files (in the current directory)
and looking for a main() function (dmake is C and C++ specific).

A simple regular expresion is used to locate main() and it will fail
for more complex incantations, e.g. using macros to define main(),
having weird arguments (Amiga) or other such silliness.

## Steps

dmake first determines the names of all the source files. By default
this is the names of the C++ (.cpp or .cc) or C (.c) files in the
current directory.

If a file named 'SRCS' exists in the current directory that file is
used instead to obtain source file names. A SRCS file contains
filenames and/or glob patterns to define the names of the source
files. The file allows filenames and patterns to occur on more than
one line and allows #-based line comments.

If dmake was invoked without one of the 'exe', 'lib' or 'dll'
arguments, dmake reads the source files looking for a main()
function. If dmake finds main() it compiles the source files
to an executable. If there is no main() dmake creates a static
library.

The output name defaults to the name of the current directory,
or if that name is "src", the name of the parent directory.
Output files are automatically prefixed and suffixed as
required, e.g. on UNIX systems static libraries have a 'lib'
prefix and '.a' suffix so a directory called "fred" will
produce "libfred.a". If the directory already has a 'lib'
prefix no extra prefix is added.

Dmake finally invokes dcc to compile the source files and
create the output.

If the 'clean' argument is supplied all output files are
removed instead of being built.

## _dmake init_
`dmake` can be run in a mode to initialize a project and create the
set of files used to control the build - the dcc _options files_ for
the project, a `.dmake` file if required, and a `Makefile` to direct
everything and provide a conventional user-experience.

### Invoking `dmake init`
The `init` _command_ is _conversational_ and accepts numerous keyword
arguments to tell it what to do. With no arguments the standard dmake
rules are used to determine the type of project and thing being
_inited_ and outputs corresponding files for dcc.

The keywords recognized by init are as follows,

 - exe | lib | dll  
Define the type of thing being built instead of inferring it.
- c | c++ | objc | objc++  
Define the programming language being used rather than
being inferred from the names of any source files.
- c99 | c11  
Define the C language standard being used. Only valid
for C language projects.
- c++11 | c++14 | c++17 | c++20  
Define the C++ language standard being used. Only
valid for C++ language projects
- debug | release  
Define the type of build to perform, debug or release (optimized).


## USAGE
    dmake [<options>] [{exe | lib | dll }] [clean]
	dmake dirs <pathname>...
    dmake init <options>...
## OPTIONS
	-C dir		Change to the named directory
			before processing. Useful when
			invoking dmake from IDEs.
	-o name		Use 'name' as the base name for the
			build output rather than the default
			based off the current directory name.
	-k		Keep going where possible, don't stop
			upon the first error.
	-v		Be more verbose and issue messages.
	-dll		When automatically creating a library,
			because no main function was found in
			the sources, create a dynamic library
			rather than a static library.
    -quiet      Pass dcc its --quiet option.

## FILES

- .dmake  
  File defining user _variables_ to define the type
  of thing being built, its name, sources and other
  build options.
- SRCS  
	Contains pathnames and glob patterns that
	expand to pathnames that define the source
	file names. Format is as per dcc "options"
	files - values written over multiple lines,
	#-style line comments and blanks ignored.

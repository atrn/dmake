# dmake - build tool using dcc

`dmake` is a tool for building C++ and C programs
using the `dcc` compiler driver. dmake determines
what is being built and invokes dcc to compile and
link, if linking is required.

Despite its name dmake is *not* a make clone and has
no relation to make or tools of its ilk. The name `dmake`
is used to provide users with a similar command to use
when building.

dmake works as a companion to the dcc compiler-driver using
it to build _modules_ of different types. Modules are either
programs, dynamic libraries or static libraries. The different
module types are distinguished by the commands used to create
them and the names of output files.

dmake uses dcc to take care of compilation. dmake determines what
and how to build. Dcc's automatic dependency-based compilation and
library generation are used ensure outputs up to date.

dmake offers some features for the lazy programmer.  It automatically
determines the name and type of artefact being built, program or library,
and invokes dcc with appropriate options.

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

## USAGE
	dmake [options] [{exe | lib | dll }] [clean]
	dmake dirs pathname...

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

- SRCS  
	Contains pathnames and glob patterns that
	expand to pathnames that define the source
	file names. Format is as per dcc "options"
	files - values written over multiple lines,
	#-style line comments and blanks ignored.

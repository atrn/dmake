# dmake - build tool using dcc

`dmake` is a build tool for C++/C that, despite its
name, is *not* a make clone. dmake has no relation
to make or other tools of its ilk.

dmake builds _modules_ and uses the dcc compiler-
driver to do take care of the compilation task.
dmake itself determines what and how to compile
and uses dcc's automatic dependency-based compiles
to keep compiled outputs up to date.

dmake offers some features to help the lazy
programmer.  dmake can automatically determine
if source code represents a program or library
and invokes dcc accordingly to create an
executable or a library.

The dmake _user-experience_ is typically a two step
thing. The first step is where we create _options files_
used by dcc when it is compiling. We usually just
create one file, CXXFLAGS, to define compiler options.
If we're building a program we may create another file,
LIBS, to link in extra libraries.

With the options files in place usage is usually
just typing `dmake`. It figures out what to do.

## Using dmake

dmake's actions depends upon how it in invoked. dmake
can either infer the type of module being built, executable
or library, or can be explicitly told the type of
module to create.

When invoked without arguments dmake attempts to infer
the type of module from the contents of the source files
(in the current directory).

dmake first determines the names of all the source files. By
default this is the names of the C++ (.cpp or .cc) or C (.c)
files in the current directory.

If a file named 'SRCS' exists in the current directory that
file is used instead to obtain source file names. A SRCS file
contains filenames and/or glob patterns to define the names of
the source files. The file allows filenames and patterns to
occur on more than one line and allows #-based line comments.

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

## FILES

- SRCS  
	Contains pathnames and glob patterns that
	expand to pathnames that define the source
	file names. Format is as per dcc "options"
	files - values written over multiple lines,
	#-style line comments and blanks ignored.

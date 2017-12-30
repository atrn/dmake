# dmake - build tool using dcc

`dmake` is a software build tool for C++/C using the
dcc compiler-driver. Despite its name dmake is *not*
a make clone and in fact bears no relation to make
or others of its ilk. dmake builds _modules_ using
dcc to do the compilation. dmake determines what
files to compile and how to compile them.

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
	dmake [-o name] [{exe | lib | dll }] [clean]
	dmake dirs pathname...
	dmake [-o name] init

## OPTIONS
	-o name		Use 'name' as the base name for the
			build output rather than the default
			based off the current directory name.

## FILES

- SRCS  
	Contains pathnames and glob patterns that
	expand to pathnames that define the source
	file names. Format is as per dcc "options"
	files - values written over multiple lines,
	#-style line comments and blanks ignored.

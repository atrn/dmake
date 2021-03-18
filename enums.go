// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import "fmt"

//  ----------------------------------------------------------------

type Action int

const (
	DefaultAction Action = iota
	Building
	Cleaning
	Initing
	Installing
)

func (a Action) String() string {
	switch a {
	case DefaultAction:
		return "default"
	case Building:
		return "build"
	case Cleaning:
		return "clean"
	case Initing:
		return "init"
	case Installing:
		return "install"
	}
	panic("unknown Action")
}

//  ----------------------------------------------------------------

type Language int

const (
	UnknownLanguage Language = iota
	CLanguage
	CplusplusLanguage
	ObjcLanguage
	ObjcplusplusLanguage
)

func (l Language) String() string {
	switch l {
	case UnknownLanguage:
		return "language not recognized"
	case CLanguage:
		return "c"
	case CplusplusLanguage:
		return "c++"
	case ObjcLanguage:
		return "objc"
	case ObjcplusplusLanguage:
		return "objc++"
	default:
		panic("unexpected language")
	}
}

func (l *Language) Set(arg string) error {
	switch arg {
	case "c":
		*l = CLanguage
	case "c++":
		*l = CplusplusLanguage
	case "objc":
		*l = ObjcLanguage
	case "objc++":
		*l = ObjcplusplusLanguage
	default:
		return fmt.Errorf("%q is not a valid language", arg)
	}
	return nil
}

//  ----------------------------------------------------------------

type OutputType int

const (
	UnknownOutputType OutputType = iota
	DllOutputType
	ExeOutputType
	LibOutputType
)

func (f OutputType) String() string {
	switch f {
	case UnknownOutputType:
		return "unknown"
	case DllOutputType:
		return "dll"
	case ExeOutputType:
		return "exe"
	case LibOutputType:
		return "lib"
	default:
		panic("unexpected OutputType")
	}
}

func (f OutputType) DccArgument() string {
	switch f {
	case DllOutputType:
		return "--dll"
	case ExeOutputType:
		return "--exe"
	case LibOutputType:
		return "--lib"
	default:
		panic("unexpected OutputType")
	}
}

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
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode"
)

//  ----------------------------------------------------------------

type Var struct {
	op    Op
	value string
}

func MakeVar(op Op, value string) Var {
	return Var{
		op:    op,
		value: value,
	}
}

func (v *Var) PlusEq(rhs Var) Var {
	return MakeVar(OpEq, v.value+rhs.value)
}

func (v *Var) MinusEq(rhs Var) Var {
	return MakeVar(OpEq, strings.ReplaceAll(v.value, rhs.value, ""))
}

//  ----------------------------------------------------------------

type Vars map[string]Var

func (vars *Vars) Set(key string, v Var) {
	(*vars)[key] = v
}

func (vars *Vars) SetValue(key, value string) {
	(*vars)[key] = MakeVar(OpEq, value)
}

func (vars *Vars) Get(key string) (Var, bool) {
	v, found := (*vars)[key]
	return v, found
}

func (vars *Vars) GetValue(key string) (string, bool) {
	if v, found := vars.Get(key); found {
		return v.value, true
	}
	return "", false
}

func (vars *Vars) GetString(key string) string {
	s, _ := vars.GetValue(key)
	return s
}

func readAndAppend(r *strings.Reader, s string, stopFn func(rune) bool) (string, error) {
	for {
		if ch, _, err := r.ReadRune(); err != nil {
			if err == io.EOF {
				err = nil
			}
			return s, err
		} else if stopFn(ch) {
			return s, nil
		} else {
			s += string(ch)
		}
	}
}

func (vars *Vars) Interpolate(s string) (string, error) {
	var b strings.Builder
	r := strings.NewReader(s)
	for {
		ch, _, err := r.ReadRune()
		if err == io.EOF {
			return b.String(), nil
		} else if err != nil {
			return b.String(), err
		}
		if ch != '$' {
			b.WriteRune(ch)
		} else {
			ch, _, err := r.ReadRune()
			if err == io.EOF {
				b.WriteRune('$')
				return b.String(), nil
			} else if err != nil {
				return b.String(), err
			}

			var key string
			if ch == '$' {
				b.WriteRune(ch)
			} else if ch == '{' {
				key, err = readAndAppend(r, "", func(ch rune) bool { return ch == '}' })
			} else {
				key, err = readAndAppend(r, string(ch), unicode.IsSpace)
			}
			if err != nil {
				return b.String(), err
			}
			b.WriteString(vars.GetString(key))
		}
	}
}

// Read a .dmake file and return a Vars containing the variables it
// defines.
//
// Variables are of the form.
//	<name> [= <value>]
//
// Names are a single, space separated, token.
//
// Values may refer to previously defined values via '$' prefixed
// names.
//
// If no value is supplied the variable is assumed to be a "boolean"
// style value and is assigned a default, string, value of "true".
//
// Blank lines and those beginning with '#' are ignored.
//
func (vars *Vars) ReadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return vars.ReadFromReader(file, path)
}

func (vars *Vars) ReadFromReader(file io.Reader, path string) error {
	var err error

	vars.SetValue("OS", runtime.GOOS)
	vars.SetValue("ARCH", runtime.GOARCH)

	lineno := 0

	fail := func(message string) error {
		return fmt.Errorf("%s:%d - %s", path, lineno, message)
	}

	for input := bufio.NewScanner(file); input.Scan(); {
		lineno++
		line := strings.TrimSpace(input.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		var key, op, val string
		opIndex := -1
		for _, op = range operators {
			opIndex = strings.Index(line, op)
			if opIndex != -1 {
				break
			}
		}
		switch opIndex {
		case -1:
			key = strings.TrimSpace(line)
			op = "="
			val = "true"
		case 0:
			return fail(fmt.Sprintf("malformed line, no variable name before %q", op))
		default:
			key = strings.TrimSpace(line[0:opIndex])
			if len(strings.Fields(key)) != 1 {
				return fail("malformed line, variable names may not contain spaces")
			}
			val = strings.TrimSpace(line[opIndex+1:])
			if val, err = vars.Interpolate(val); err != nil {
				return err
			}
		}
		vars.Apply(key, Var{OpFromString(op), val})
	}

	return nil
}

func (vars *Vars) Apply(key string, rhs Var) {
	lhs, found := vars.Get(key)
	switch rhs.op {
	case OpEq:
		vars.Set(key, rhs)
	case OpPlusEq:
		if found {
			vars.Set(key, lhs.PlusEq(rhs))
		} else {
			vars.SetValue(key, rhs.value)
		}
	case OpMinusEq:
		if found {
			vars.Set(key, lhs.MinusEq(rhs))
		} else {
			vars.SetValue(key, rhs.value)
		}
	default:
		panic(fmt.Errorf("unexpected operator - %q", rhs.op.String()))
	}
}

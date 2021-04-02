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
	"os"
	"runtime"
	"strings"
)

type Vars map[string]string

func (vars *Vars) Set(key, value string) {
	(*vars)[key] = value
}

func (vars *Vars) Get(key string) (value string, found bool) {
	value, found = (*vars)[key]
	return
}

func (vars *Vars) InterpolateVars(s string) string {
	r := strings.Fields(s)
	for index, word := range r {
		if word[0] == '$' {
			key := word[1:]
			if value, found := vars.Get(key); found {
				r[index] = value
			}
		}
	}
	return strings.Join(r, " ")
}

// Read a .dmake and return a Vars containing the variables it
// defines.
//
// Variables are of the form <name> = <value>, names is a single,
// space separated, token. Values may refer to previously defined
// values via '$' prefixed names.  Blank lines and those beginning
// with '#' are ignored.
//
func (vars *Vars) ReadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	vars.Set("OS", runtime.GOOS)
	vars.Set("ARCH", runtime.GOARCH)

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
		val = vars.InterpolateVars(val)
		vars.Set(key, val)
	}

	return nil
}

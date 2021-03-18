// dmake - a build tool on top of dcc
//
// Copyright (C) 2017 A.Newman.
//
// This source code is released under version 2 of the GNU Public
// License.  See the file LICENSE for details.
//

package main

import "strings"

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

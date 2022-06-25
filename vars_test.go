package main

import (
	"strings"
	"testing"
)

func TestReadFromFile(t *testing.T) {
	goodInput := `# A comment

AVAR = AVAR-Value
BOOLEAN

# EOF
`

	vars := make(Vars)
	r := strings.NewReader(goodInput)
	err := vars.ReadFromReader(r, goodInput)
	if err != nil {
		t.Fatal(err)
	}

	check := func(key, expectedValue string) {
		actualValue, found := vars.GetValue(key)
		if !found {
			t.Fatalf("variable %s not defined", key)
		}
		if actualValue != expectedValue {
			t.Fatalf("variable %s has value %q, expected %q", key, actualValue, expectedValue)
		}
	}

	check("AVAR", "AVAR-Value")
	check("BOOLEAN", "true")
}

func TestInterpolate(t *testing.T) {
	vars := make(Vars)
	vars.SetValue("AVAR", "AVAR_VALUE")
	vars.SetValue("BVAR", "BVAR_VALUE")

	interpolate := func(s string) string {
		r, err := vars.Interpolate(s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	//  empty string -> empty string
	s := interpolate("")
	if s != "" {
		t.Fail()
	}

	//  nothing to interpolate -> input
	s = interpolate("NOMATCH")
	if s != "NOMATCH" {
		t.Fail()
	}

	//  $$ -> $

	s = interpolate("$$")
	if s != "$" {
		t.Fail()
	}
	s = interpolate("PREFIX$$")
	if s != "PREFIX$" {
		t.Fail()
	}
	s = interpolate("$$SUFFIX")
	if s != "$SUFFIX" {
		t.Fail()
	}
	s = interpolate("PREFIX$$SUFFIX")
	if s != "PREFIX$SUFFIX" {
		t.Fail()
	}

	// non-existent variable replaced by empty string
	s = interpolate("$NOVAR")
	if s != "" {
		t.Fail()
	}

	s = interpolate("${NOVAR}")
	if s != "" {
		t.Fail()
	}

	s = interpolate("PREFIX${NOVAR}")
	if s != "PREFIX" {
		t.Fail()
	}

	s = interpolate("PREFIX$NOVAR")
	if s != "PREFIX" {
		t.Fail()
	}

	s = interpolate("${NOVAR}SUFFIX")
	if s != "SUFFIX" {
		t.Fail()
	}

	s = interpolate("$NOVARSUFFIX")
	if s != "" {
		t.Fail()
	}

	s = interpolate("PREFIX${NOVAR}SUFFIX")
	if s != "PREFIXSUFFIX" {
		t.Fail()
	}

	// Varaiable replaced by its value
	//
	s = interpolate("${AVAR}")
	if s != "AVAR_VALUE" {
		t.Fail()
	}

	s = interpolate("PREFIX_${AVAR}_SUFFIX")
	if s != "PREFIX_AVAR_VALUE_SUFFIX" {
		t.Fail()
	}

	s = interpolate("PREFIX_${AVAR}_SUFFIX_${AVAR}")
	if s != "PREFIX_AVAR_VALUE_SUFFIX_AVAR_VALUE" {
		t.Fail()
	}

	s = interpolate("PREFIX_${AVAR}")
	if s != "PREFIX_AVAR_VALUE" {
		t.Fail()
	}

	s = interpolate("${AVAR}_SUFFIX")
	if s != "AVAR_VALUE_SUFFIX" {
		t.Fail()
	}

	// More than one variable
	s = interpolate("${AVAR}$BVAR")
	if s != "AVAR_VALUEBVAR_VALUE" {
		t.Fail()
	}
	s = interpolate("${AVAR}${BVAR}")
	if s != "AVAR_VALUEBVAR_VALUE" {
		t.Fail()
	}
}

func TestOps(t *testing.T) {
	vars := make(Vars)
	vars.SetValue("AVAR", "AVAR_VALUE")

	vars.Apply("AVAR", MakeVar(OpPlusEq, "_CVAR_VALUE"))
	if vars.GetString("AVAR") != "AVAR_VALUE_CVAR_VALUE" {
		t.Fail()
	}

	vars.Apply("AVAR", MakeVar(OpMinusEq, "A"))
	if vars.GetString("AVAR") != "VR_VLUE_CVR_VLUE" {
		t.Fail()
	}

}

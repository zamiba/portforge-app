package main

import (
	"strings"
	"testing"

	"github.com/zamiba/forge/engine"
)

func TestCheckProviderRefsKnowsWhereEachProviderExists(t *testing.T) {
	spec := func(steps ...engine.Step) *engine.Spec { return &engine.Spec{Steps: steps} }
	exe := func(args ...string) engine.Step {
		return engine.Step{Step: "defineExecutable", Executable: "install/game", Title: "Play", Args: args}
	}
	cases := []struct {
		name string
		spec *engine.Spec
		want string // substring of the error; empty means accepted
	}{
		{"rom during the run", spec(engine.Step{Step: "copy", From: "rom", Dest: "install/rom.z64"}, exe()), ""},
		{"profile at launch", spec(exe("--savepath", "${profilePath}")), ""},
		{"rom at launch", spec(exe("${romPath}")), ""},
		{"profile during the run", spec(engine.Step{Step: "createDir", Path: "${profilePath}/saves"}, exe()), "reads ${profilePath}"},
		{"unknown at launch", spec(exe("${savesPath}")), "launches with ${savesPath}"},
		{"unknown during the run", spec(engine.Step{Step: "touch", Path: "${homePath}/x"}, exe()), "reads ${homePath}"},
	}
	for _, tc := range cases {
		err := checkProviderRefs(tc.spec)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: got %v, want an error mentioning %q", tc.name, err, tc.want)
		}
	}
}

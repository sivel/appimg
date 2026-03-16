package cmd

import "testing"

func TestBuildVersion_LdflagsSet(t *testing.T) {
	orig := version
	version = "v1.2.3"
	t.Cleanup(func() { version = orig })

	got := buildVersion()
	if got != "v1.2.3" {
		t.Errorf("buildVersion() = %q, want v1.2.3", got)
	}
}

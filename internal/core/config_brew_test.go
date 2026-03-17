package core

import (
	"testing"
)

func TestConfigMap_Get_NotFound(t *testing.T) {
	cm := ConfigMap{}
	if got := cm.Get("missing"); got != nil {
		t.Errorf("Get(missing) = %v, want nil", got)
	}
}

func TestConfigMap_Get_NoIncludes(t *testing.T) {
	cm := ConfigMap{
		"simple": {
			Taps:  []string{"tap1"},
			Brews: []string{"pkg1"},
		},
	}

	got := cm.Get("simple")
	if got == nil {
		t.Fatal("Get(simple) = nil")
	}
	if len(got.Taps) != 1 || got.Taps[0] != "tap1" {
		t.Errorf("Taps = %v, want [tap1]", got.Taps)
	}
	if len(got.Brews) != 1 || got.Brews[0] != "pkg1" {
		t.Errorf("Brews = %v, want [pkg1]", got.Brews)
	}
}

func TestConfigMap_Get_WithIncludes(t *testing.T) {
	cm := ConfigMap{
		"base": {
			Brews: []string{"curl"},
			Taps:  []string{"base/tap"},
		},
		"extended": {
			Includes: []string{"base"},
			Brews:    []string{"git"},
			Casks:    []string{"firefox"},
		},
	}

	got := cm.Get("extended")
	if got == nil {
		t.Fatal("Get(extended) = nil")
	}

	// Included items come first, then the base config's own items
	wantBrews := []string{"curl", "git"}
	if len(got.Brews) != len(wantBrews) {
		t.Fatalf("Brews = %v, want %v", got.Brews, wantBrews)
	}
	for i, want := range wantBrews {
		if got.Brews[i] != want {
			t.Errorf("Brews[%d] = %q, want %q", i, got.Brews[i], want)
		}
	}

	if len(got.Taps) != 1 || got.Taps[0] != "base/tap" {
		t.Errorf("Taps = %v, want [base/tap]", got.Taps)
	}
	if len(got.Casks) != 1 || got.Casks[0] != "firefox" {
		t.Errorf("Casks = %v, want [firefox]", got.Casks)
	}
}

func TestConfigMap_Get_CircularIncludes(t *testing.T) {
	cm := ConfigMap{
		"a": {
			Includes: []string{"b"},
			Brews:    []string{"pkg-a"},
		},
		"b": {
			Includes: []string{"a"},
			Brews:    []string{"pkg-b"},
		},
	}

	// Should not infinite loop; circular reference is broken
	got := cm.Get("a")
	if got == nil {
		t.Fatal("Get(a) = nil")
	}

	// "b" is included into "a", but "b" trying to include "a" is skipped
	wantBrews := []string{"pkg-b", "pkg-a"}
	if len(got.Brews) != len(wantBrews) {
		t.Fatalf("Brews = %v, want %v", got.Brews, wantBrews)
	}
	for i, want := range wantBrews {
		if got.Brews[i] != want {
			t.Errorf("Brews[%d] = %q, want %q", i, got.Brews[i], want)
		}
	}
}

func TestConfigMap_Get_PreservesRemove(t *testing.T) {
	cm := ConfigMap{
		"cleanup": {
			Remove: true,
			Brews:  []string{"oldpkg"},
		},
	}

	got := cm.Get("cleanup")
	if got == nil {
		t.Fatal("Get(cleanup) = nil")
	}
	if !got.Remove {
		t.Error("Remove = false, want true")
	}
}

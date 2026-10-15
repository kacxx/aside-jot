package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The release PR bumps these versions through extra-files in
// release-please-config.json. If a jsonpath is wrong or a file moves, plugin
// updates stop silently, so keep the three in step here.
func TestPluginVersionsMatchRelease(t *testing.T) {
	read := func(path string, v any) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}

	var manifest map[string]string
	read(".release-please-manifest.json", &manifest)
	release := manifest["."]
	if release == "" {
		t.Fatal(`.release-please-manifest.json has no "." version`)
	}

	var plugin struct {
		Version string `json:"version"`
	}
	read(".claude-plugin/plugin.json", &plugin)
	if plugin.Version != release {
		t.Errorf("plugin.json version = %q, release-please manifest = %q", plugin.Version, release)
	}

	var market struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Ref string `json:"ref"`
			} `json:"source"`
		} `json:"plugins"`
	}
	read(".claude-plugin/marketplace.json", &market)
	ref, found := "", false
	for _, p := range market.Plugins {
		if p.Name == "aside" {
			ref, found = p.Source.Ref, true
		}
	}
	if !found {
		t.Fatal(`marketplace.json has no "aside" plugin`)
	}
	if want := "v" + release; ref != want {
		t.Errorf("marketplace.json aside ref = %q, want %q", ref, want)
	}
}

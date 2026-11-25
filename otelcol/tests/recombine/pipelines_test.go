package recombinecheck

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// Fragments are merged with lists replaced, so two fragments defining the same pipeline
// with different receivers silently drop one of them. Sharing a pipeline is only fine
// when both definitions are identical (entity_metrics + container_metrics).
func TestPipelineNamesDoNotCollide(t *testing.T) {
	files, _ := filepath.Glob("../../rootfs/etc/otelcol/*.yaml")
	type pipe struct {
		Receivers []string `yaml:"receivers"`
	}
	seen := map[string]struct {
		file      string
		receivers []string
	}{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Service struct {
				Pipelines map[string]pipe `yaml:"pipelines"`
			} `yaml:"service"`
		}
		if err := yaml.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for name, p := range c.Service.Pipelines {
			if p.Receivers == nil {
				continue // only overrides processors (logs_parse.yaml)
			}
			if prev, ok := seen[name]; ok && !slices.Equal(prev.receivers, p.Receivers) {
				t.Errorf("pipeline %q: %s has receivers %v, %s has %v; the last one loaded wins",
					name, prev.file, prev.receivers, filepath.Base(f), p.Receivers)
			}
			seen[name] = struct {
				file      string
				receivers []string
			}{filepath.Base(f), p.Receivers}
		}
	}
}

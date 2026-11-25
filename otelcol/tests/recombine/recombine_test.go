// Checks the journald recombine operator in rootfs/etc/otelcol/logs_multiline.yaml: a Python
// traceback becomes one record; host lines and other containers are never joined.
package recombinecheck

import (
	"os"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/transformer/recombine"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/testutil"
	"go.opentelemetry.io/collector/component/componenttest"
	"gopkg.in/yaml.v3"
)

func TestTraceback(t *testing.T) {
	raw, _ := os.ReadFile("../../rootfs/etc/otelcol/logs_multiline.yaml")
	var c struct {
		Receivers struct {
			Journald struct {
				Operators []struct {
					Type         string `yaml:"type"`
					IsFirstEntry string `yaml:"is_first_entry"`
					CombineWith  string `yaml:"combine_with"`
				} `yaml:"operators"`
			} `yaml:"journald"`
		} `yaml:"receivers"`
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	ops := c.Receivers.Journald.Operators
	o := ops[len(ops)-1]
	if o.Type != "recombine" {
		t.Fatalf("last operator is %q, want recombine", o.Type)
	}
	cfg := recombine.NewConfig()
	cfg.CombineField = entry.NewBodyField("MESSAGE")
	cfg.SourceIdentifier = entry.NewBodyField("SYSLOG_IDENTIFIER")
	cfg.IsFirstEntry = o.IsFirstEntry
	cfg.CombineWith = o.CombineWith
	cfg.ForceFlushTimeout = 100 * time.Millisecond
	cfg.OutputIDs = []string{"fake"}
	op, err := cfg.Build(componenttest.NewNopTelemetrySettings())
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Start(testutil.NewUnscopedMockPersister()); err != nil {
		t.Fatal(err)
	}
	fake := testutil.NewFakeOutput(t)
	_ = op.SetOutputs([]operator.Operator{fake})
	mk := func(c, m string) *entry.Entry {
		e := entry.New()
		b := map[string]any{"MESSAGE": m, "SYSLOG_IDENTIFIER": "kernel"}
		if c != "" {
			b["CONTAINER_NAME"] = c
			b["SYSLOG_IDENTIFIER"] = c + "-id"
		}
		e.Body = b
		return e
	}
	in := []*entry.Entry{
		mk("homeassistant", "2026-11-25 10:00:00.000 ERROR (MainThread) [x] Error doing job"),
		mk("homeassistant", "Traceback (most recent call last):"),
		mk("homeassistant", `  File "/x.py", line 1, in f`),
		mk("homeassistant", "    boom()"),
		mk("homeassistant", "ValueError: boom"),
		mk("", "    indented kernel line"),
		mk("homeassistant", "2026-11-25 10:00:01.000 INFO (MainThread) [y] next"),
		mk("addon_x", "plain line"),
	}
	for _, e := range in {
		if err := op.ProcessBatch(t.Context(), []*entry.Entry{e}); err != nil {
			t.Fatal(err)
		}
	}
	_ = op.Stop()
	var got []string
	for len(fake.Received) > 0 {
		e := <-fake.Received
		got = append(got, e.Body.(map[string]any)["MESSAGE"].(string))
	}
	for _, g := range got {
		t.Logf("%q", g)
	}
	want := "2026-11-25 10:00:00.000 ERROR (MainThread) [x] Error doing job\nTraceback (most recent call last):\n  File \"/x.py\", line 1, in f\n    boom()\nValueError: boom"
	found := false
	for _, g := range got {
		found = found || g == want
	}
	if !found || len(got) != 4 {
		t.Fatalf("want 4 records incl. the joined traceback, got %d", len(got))
	}
}

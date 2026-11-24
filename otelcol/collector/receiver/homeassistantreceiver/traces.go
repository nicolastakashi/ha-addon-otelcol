package homeassistantreceiver

import (
	"crypto/sha256"
	"math/big"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// traceSummary is one entry of the trace/list WebSocket command.
type traceSummary struct {
	Domain string `json:"domain"`
	ItemID string `json:"item_id"`
	RunID  string `json:"run_id"`
	State  string `json:"state"`
}

// traceDetail is the trace/get WebSocket result (homeassistant/components/trace).
type traceDetail struct {
	Domain          string `json:"domain"`
	ItemID          string `json:"item_id"`
	RunID           string `json:"run_id"`
	State           string `json:"state"`
	ScriptExecution string `json:"script_execution"`
	Error           string `json:"error"`
	Trigger         string `json:"trigger"`
	Timestamp       struct {
		Start  time.Time `json:"start"`
		Finish time.Time `json:"finish"`
	} `json:"timestamp"`
	Context struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
	Config struct {
		Alias string `json:"alias"`
	} `json:"config"`
	Trace map[string][]traceStep `json:"trace"`
}

type traceStep struct {
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error"`
	ChildID   *struct {
		Domain string `json:"domain"`
		ItemID string `json:"item_id"`
		RunID  string `json:"run_id"`
	} `json:"child_id"`
}

// toTraces maps one finished run to a root span plus one child span per step.
// Home Assistant records only when each step starts, so a step ends when the
// next one starts (or when the run finishes).
func toTraces(d traceDetail) ptrace.Traces {
	td := ptrace.NewTraces()
	ss := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	ss.Scope().SetName("github.com/nicolastakashi/ha-addon-otelcol/homeassistantreceiver")

	start, finish := d.Timestamp.Start, d.Timestamp.Finish
	if finish.Before(start) {
		finish = start
	}
	traceID := traceIDFor(d.Context.ID, d.RunID)

	root := ss.Spans().AppendEmpty()
	root.SetTraceID(traceID)
	root.SetSpanID(spanIDFor(d.RunID, "root"))
	name := d.ItemID
	if d.Config.Alias != "" {
		name = d.Config.Alias
	}
	root.SetName(d.Domain + " " + name)
	root.SetKind(ptrace.SpanKindInternal)
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(finish))
	a := root.Attributes()
	a.PutStr("homeassistant.domain", d.Domain)
	a.PutStr("homeassistant.item_id", d.ItemID)
	a.PutStr("homeassistant.run_id", d.RunID)
	putIf(a, "homeassistant.alias", d.Config.Alias)
	putIf(a, "homeassistant.trigger", d.Trigger)
	putIf(a, "homeassistant.script_execution", d.ScriptExecution)
	putIf(a, "homeassistant.context.id", d.Context.ID)
	putIf(a, "homeassistant.context.parent_id", d.Context.ParentID)
	putIf(a, "homeassistant.context.user_id", d.Context.UserID)
	if d.Error != "" {
		root.Status().SetCode(ptrace.StatusCodeError)
		root.Status().SetMessage(d.Error)
	}

	var steps []traceStep
	for _, s := range d.Trace {
		steps = append(steps, s...)
	}
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].Timestamp.Equal(steps[j].Timestamp) {
			return steps[i].Path < steps[j].Path
		}
		return steps[i].Timestamp.Before(steps[j].Timestamp)
	})
	for i, s := range steps {
		end := finish
		if i+1 < len(steps) && steps[i+1].Timestamp.After(s.Timestamp) {
			end = steps[i+1].Timestamp
		}
		if end.Before(s.Timestamp) {
			end = s.Timestamp
		}
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(spanIDFor(d.RunID, s.Path+"#"+s.Timestamp.Format(time.RFC3339Nano)))
		sp.SetParentSpanID(root.SpanID())
		sp.SetName(stepName(s.Path))
		sp.SetKind(ptrace.SpanKindInternal)
		sp.SetStartTimestamp(pcommon.NewTimestampFromTime(s.Timestamp))
		sp.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
		sp.Attributes().PutStr("homeassistant.step.path", s.Path)
		if s.ChildID != nil {
			sp.Attributes().PutStr("homeassistant.step.child", s.ChildID.Domain+"."+s.ChildID.ItemID+"/"+s.ChildID.RunID)
		}
		if s.Error != "" {
			sp.Status().SetCode(ptrace.StatusCodeError)
			sp.Status().SetMessage(s.Error)
		}
	}
	return td
}

// stepName turns "action/0/choose/1/sequence/0" into "action choose sequence".
func stepName(path string) string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" && (p[0] < '0' || p[0] > '9') {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return path
	}
	return strings.Join(parts, " ")
}

func putIf(m pcommon.Map, k, v string) {
	if v != "" {
		m.PutStr(k, v)
	}
}

// traceIDFor uses the Home Assistant context id (a ULID, 128 bits) so the
// trace id is stable and can be correlated; it falls back to a hash of the run id.
func traceIDFor(contextID, runID string) pcommon.TraceID {
	if b, ok := ulidBytes(contextID); ok {
		return pcommon.TraceID(b)
	}
	sum := sha256.Sum256([]byte(runID))
	var id pcommon.TraceID
	copy(id[:], sum[:16])
	return id
}

func spanIDFor(runID, key string) pcommon.SpanID {
	sum := sha256.Sum256([]byte(runID + "/" + key))
	var id pcommon.SpanID
	copy(id[:], sum[:8])
	return id
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func ulidBytes(s string) ([16]byte, bool) {
	var out [16]byte
	if len(s) != 26 {
		return out, false
	}
	v := new(big.Int)
	for _, c := range strings.ToUpper(s) {
		i := strings.IndexRune(crockford, c)
		if i < 0 {
			return out, false
		}
		v.Lsh(v, 5).Or(v, big.NewInt(int64(i)))
	}
	if v.BitLen() > 128 || v.Sign() == 0 {
		return out, false
	}
	v.FillBytes(out[:])
	return out, true
}

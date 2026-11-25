package homeassistantreceiver

import (
	"crypto/sha256"
	"math/big"
	"sort"
	"strconv"
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
	// The automation/script config at the time of the run; steps resolve their alias here.
	Config map[string]any         `json:"config"`
	Trace  map[string][]traceStep `json:"trace"`
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
	alias, _ := d.Config["alias"].(string)
	name := d.ItemID
	if alias != "" {
		name = alias
	}
	root.SetName(d.Domain + " " + name)
	root.SetKind(ptrace.SpanKindInternal)
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(finish))
	a := root.Attributes()
	a.PutStr("homeassistant.domain", d.Domain)
	a.PutStr("homeassistant.item_id", d.ItemID)
	a.PutStr("homeassistant.run_id", d.RunID)
	putIf(a, "homeassistant.alias", alias)
	putIf(a, "homeassistant.trigger", d.Trigger)
	putIf(a, "homeassistant.script_execution", d.ScriptExecution)
	putIf(a, "homeassistant.context.id", d.Context.ID)
	putIf(a, "homeassistant.context.parent_id", d.Context.ParentID)
	putIf(a, "user.id", d.Context.UserID) // semconv user.id
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
	// Steps nest by path: "action/1/if/condition/0" goes under the latest "action/1/if" or
	// "action/1" step. A step ends when the next step outside its subtree starts.
	latest := map[string]pcommon.SpanID{}
	for i, s := range steps {
		end := finish
		for _, n := range steps[i+1:] {
			if !strings.HasPrefix(n.Path, s.Path+"/") {
				if n.Timestamp.After(s.Timestamp) {
					end = n.Timestamp
				}
				break
			}
		}
		if end.Before(s.Timestamp) {
			end = s.Timestamp
		}
		parent := root.SpanID()
		for p := s.Path; strings.Contains(p, "/"); {
			p = p[:strings.LastIndex(p, "/")]
			if id, ok := latest[p]; ok {
				parent = id
				break
			}
		}
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(spanIDFor(d.RunID, s.Path+"#"+s.Timestamp.Format(time.RFC3339Nano)))
		latest[s.Path] = sp.SpanID()
		sp.SetParentSpanID(parent)
		sp.SetName(stepName(s.Path, d.Config))
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

// stepName names a step after the last part of its path plus its index, and what it does
// when the config says: "action 2: Ligar resistência", "action 3: switch.turn_on",
// "condition 0: state", "if".
func stepName(path string, config map[string]any) string {
	segs := strings.Split(path, "/")
	kind, idx := path, ""
	for i := len(segs) - 1; i >= 0; i-- {
		if !isIndex(segs[i]) {
			kind = segs[i]
			if i+1 < len(segs) {
				idx = " " + segs[i+1]
			}
			break
		}
	}
	name := kind + idx
	found := lookup(config, segs)
	if v, ok := found.(string); ok && v != "" { // e.g. entity_id/0 of a condition
		return name + ": " + v
	}
	node, _ := found.(map[string]any)
	for _, k := range []string{"alias", "action", "service", "condition", "trigger", "platform"} {
		if v, ok := node[k].(string); ok && v != "" {
			return name + ": " + v
		}
	}
	// No alias: structural steps are named by their type ("action 0: choose").
	for _, k := range []string{"choose", "if", "repeat", "parallel", "sequence", "wait_template", "wait_for_trigger", "delay", "variables", "stop"} {
		if _, ok := node[k]; ok {
			return name + ": " + k
		}
	}
	return name
}

// lookup walks a step path through the run's config. Paths use singular keys ("action/1")
// where the config may use plural ones ("actions"); a name segment over a list
// ("if/condition/0", the list of if-conditions) is only a label and is skipped.
func lookup(node any, segs []string) any {
	for _, s := range segs {
		switch n := node.(type) {
		case map[string]any:
			v, ok := n[s]
			if !ok {
				v, ok = n[s+"s"]
			}
			if !ok {
				return nil
			}
			node = v
		case []any:
			if !isIndex(s) {
				continue
			}
			i, err := strconv.Atoi(s)
			if err != nil || i >= len(n) {
				return nil
			}
			node = n[i]
		case string: // a single value where HA accepts a list too: entity_id: x == [x]
			if s != "0" {
				return nil
			}
		default:
			return nil
		}
	}
	return node
}

func isIndex(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
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

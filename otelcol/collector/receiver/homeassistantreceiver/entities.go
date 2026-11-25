package homeassistantreceiver

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type haState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastUpdated time.Time      `json:"last_updated"`
}

type entityEntry struct {
	EntityID string `json:"entity_id"`
	DeviceID string `json:"device_id"`
	AreaID   string `json:"area_id"`
	Platform string `json:"platform"`
}

type deviceEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	NameByUser   string `json:"name_by_user"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	AreaID       string `json:"area_id"`
}

type areaEntry struct {
	AreaID  string `json:"area_id"`
	Name    string `json:"name"`
	FloorID string `json:"floor_id"`
}

type floorEntry struct {
	FloorID string `json:"floor_id"`
	Name    string `json:"name"`
}

// numericDomains are domains whose numeric state is a measurement or a setting.
var numericDomains = map[string]bool{"sensor": true, "number": true, "input_number": true, "counter": true}

// entityMetrics reads every entity state plus the entity, device, area and floor
// registries over the WebSocket API and turns them into gauges:
//
//	homeassistant.sensor.<device_class>  numeric sensor state (unit from the entity)
//	homeassistant.<domain>.value         numeric state of other numeric entities
//	homeassistant.<domain>.on            1/0 for on/off entities
//	homeassistant.entity.state           1 with homeassistant.entity.state=<text state>
//	homeassistant.<domain>.attribute.<k> numeric state attributes (entity_attributes)
//	homeassistant.entity.available       0 when unavailable, else 1
//	homeassistant.entity.last_updated    Unix seconds of the entity's last update
//	homeassistant.core.up                1 when Core answered this scrape, else 0
func (r *statsReceiver) entityMetrics(ctx context.Context, ms pmetric.MetricSlice, now pcommon.Timestamp) (err error) {
	up := gauge(ms, MetricHomeassistantCoreUp, "1", "1 when Home Assistant Core answered the WebSocket API at this scrape, else 0.")
	defer func() {
		v := 1.0
		if err != nil {
			v = 0
		}
		add(up, now, pcommon.NewMap(), v)
	}()
	c, err := dial(ctx, r.cfg.Endpoint, string(r.cfg.Token))
	if err != nil {
		return err
	}
	defer c.close()
	var (
		states  []haState
		ents    []entityEntry
		devices []deviceEntry
		areas   []areaEntry
		floors  []floorEntry
	)
	if err := c.call(ctx, map[string]any{"type": "get_states"}, &states); err != nil {
		return err
	}
	// Registries only add context; a failure (old Core, missing floors) is not fatal.
	_ = c.call(ctx, map[string]any{"type": "config/entity_registry/list"}, &ents)
	_ = c.call(ctx, map[string]any{"type": "config/device_registry/list"}, &devices)
	_ = c.call(ctx, map[string]any{"type": "config/area_registry/list"}, &areas)
	_ = c.call(ctx, map[string]any{"type": "config/floor_registry/list"}, &floors)

	entByID := map[string]entityEntry{}
	for _, e := range ents {
		entByID[e.EntityID] = e
	}
	devByID := map[string]deviceEntry{}
	for _, d := range devices {
		devByID[d.ID] = d
	}
	areaByID := map[string]areaEntry{}
	for _, a := range areas {
		areaByID[a.AreaID] = a
	}
	floorByID := map[string]string{}
	for _, f := range floors {
		floorByID[f.FloorID] = f.Name
	}

	metrics := map[string]pmetric.Metric{} // name + unit -> metric
	get := func(name, unit string) pmetric.Metric {
		if m, ok := metrics[name+"\x00"+unit]; ok {
			return m
		}
		m := gauge(ms, name, unit, "")
		metrics[name+"\x00"+unit] = m
		return m
	}

	for _, s := range states {
		if r.excluded(s.EntityID) {
			continue
		}
		domain, _, _ := strings.Cut(s.EntityID, ".")
		attrs := pcommon.NewMap()
		attrs.PutStr(AttrHomeassistantEntityId, s.EntityID)
		attrs.PutStr(AttrHomeassistantEntityDomain, domain)
		putAttr(attrs, AttrHomeassistantEntityName, s.Attributes["friendly_name"])
		putAttr(attrs, AttrHomeassistantEntityDeviceClass, s.Attributes["device_class"])
		e := entByID[s.EntityID]
		putAttr(attrs, AttrHomeassistantEntityPlatform, e.Platform)
		areaID := e.AreaID
		if d, ok := devByID[e.DeviceID]; ok {
			name := d.NameByUser
			if name == "" {
				name = d.Name
			}
			putAttr(attrs, AttrHomeassistantDeviceName, name)
			putAttr(attrs, AttrHomeassistantDeviceManufacturer, d.Manufacturer)
			putAttr(attrs, AttrHomeassistantDeviceModel, d.Model)
			if areaID == "" {
				areaID = d.AreaID
			}
		}
		if a, ok := areaByID[areaID]; ok {
			putAttr(attrs, AttrHomeassistantAreaName, a.Name)
			putAttr(attrs, AttrHomeassistantFloorName, floorByID[a.FloorID])
		}

		available := 1.0
		if s.State == "unavailable" {
			available = 0
		}
		add(get(MetricHomeassistantEntityAvailable, "1"), now, attrs, available)
		if !s.LastUpdated.IsZero() {
			add(get(MetricHomeassistantEntityLastUpdated, "s"), now, attrs, float64(s.LastUpdated.UnixNano())/1e9)
		}

		if r.cfg.EntityAttributes {
			for k, v := range s.Attributes {
				if f, ok := v.(float64); ok && !matchAny(r.cfg.ExcludeAttributes, k) {
					add(get("homeassistant."+domain+".attribute."+k, ""), now, attrs, f)
				}
			}
		}

		switch {
		case s.State == "on" || s.State == "off":
			v := 0.0
			if s.State == "on" {
				v = 1
			}
			add(get("homeassistant."+domain+".on", "1"), now, attrs, v)
		case s.State == "unavailable" || s.State == "unknown" || s.State == "":
		default:
			v, err := strconv.ParseFloat(s.State, 64)
			if err == nil && numericDomains[domain] {
				unit, _ := s.Attributes["unit_of_measurement"].(string)
				name := "homeassistant." + domain + ".value"
				if dc, _ := s.Attributes["device_class"].(string); domain == "sensor" && dc != "" {
					name = "homeassistant.sensor." + dc
				}
				add(get(name, unit), now, attrs, v)
				continue
			}
			// A text state (heat, home, playing...): 1 with the state as an attribute. Only
			// enumerable states, so free text, timestamps, versions and numbers don't create a
			// series per value.
			if !enumerableState(domain, s.State) || err == nil {
				continue
			}
			st := pcommon.NewMap()
			attrs.CopyTo(st)
			st.PutStr(AttrHomeassistantEntityState, s.State)
			add(get(MetricHomeassistantEntityState, "1"), now, st, 1)
		}
	}
	return nil
}

// enumerableState reports whether a text state comes from a small fixed set: an
// input_select option, or a lowercase word such as idle, open, not_home, above_horizon.
func enumerableState(domain, state string) bool {
	switch domain {
	case "input_select", "select":
		return true
	case "input_text", "text":
		return false
	}
	for _, c := range state {
		if (c < 'a' || c > 'z') && c != '_' {
			return false
		}
	}
	return true
}

// matchAny reports whether s matches one of the glob patterns (path.Match).
func matchAny(patterns []string, s string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, s); ok {
			return true
		}
	}
	return false
}

func (r *statsReceiver) excluded(entityID string) bool {
	return matchAny(r.cfg.ExcludeEntities, entityID)
}

func putAttr(m pcommon.Map, k string, v any) {
	if s, ok := v.(string); ok && s != "" {
		m.PutStr(k, s)
	}
}

func add(m pmetric.Metric, now pcommon.Timestamp, attrs pcommon.Map, v float64) {
	dp := m.Gauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(now)
	attrs.CopyTo(dp.Attributes())
	dp.SetDoubleValue(v)
}

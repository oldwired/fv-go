// Package serial provides a JSON-serialization registry for fv-go views
// and the small set of helpers FV uses to round-trip Point/Rect/StringList.
//
// The registry maps a TypeID string ("button", "window", ...) to a
// factory that builds a fresh, empty Serializable. Each ported view
// type registers itself in init(). Deserialize reads "typeId" from a
// JSON object, looks up the factory, calls FromJSON on the result.
//
// Ported from FVSerialization.pas. The Pascal version uses TJSONObject
// from System.JSON; we use encoding/json with json.RawMessage to keep
// a similar shape (each Serializable handles its own subtree).
package serial

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Serializable is implemented by every view type that wants to round-trip
// to/from JSON. The TypeID returned by GetTypeID is the registry key.
type Serializable interface {
	GetTypeID() string
	ToJSON() (json.RawMessage, error)
	FromJSON(json.RawMessage) error
}

// Factory builds a fresh, zero-state Serializable. After Deserialize
// looks up the factory and constructs the empty value, it calls
// FromJSON to populate fields.
type Factory func() Serializable

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register associates typeID with factory. Safe to call from init().
// Overwrites any prior registration.
func Register(typeID string, factory Factory) {
	registryMu.Lock()
	registry[typeID] = factory
	registryMu.Unlock()
}

// CanCreate reports whether typeID has a registered factory.
func CanCreate(typeID string) bool {
	registryMu.RLock()
	_, ok := registry[typeID]
	registryMu.RUnlock()
	return ok
}

// Create returns a fresh, empty Serializable for typeID, or an error
// if no factory is registered.
func Create(typeID string) (Serializable, error) {
	registryMu.RLock()
	f, ok := registry[typeID]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("serial: no factory registered for type %q", typeID)
	}
	return f(), nil
}

// RegisteredTypes returns the sorted list of registered TypeIDs.
// Useful for tests and diagnostics.
func RegisteredTypes() []string {
	registryMu.RLock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	registryMu.RUnlock()
	sort.Strings(out)
	return out
}

// envelope is the wire shape of any Serializable. The outer JSON object
// contains a "typeId" discriminator plus the type-specific payload in
// "data".
type envelope struct {
	TypeID string          `json:"typeId"`
	Data   json.RawMessage `json:"data"`
}

// Serialize returns the JSON envelope for s.
func Serialize(s Serializable) ([]byte, error) {
	data, err := s.ToJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{TypeID: s.GetTypeID(), Data: data})
}

// Deserialize reads a JSON envelope, looks up the factory, and returns
// the populated Serializable.
func Deserialize(raw []byte) (Serializable, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	s, err := Create(env.TypeID)
	if err != nil {
		return nil, err
	}
	if err := s.FromJSON(env.Data); err != nil {
		return nil, err
	}
	return s, nil
}

// PointToJSON returns the canonical JSON form of a geom.Point.
func PointToJSON(p geom.Point) json.RawMessage {
	b, _ := json.Marshal(struct {
		X int `json:"x"`
		Y int `json:"y"`
	}{p.X, p.Y})
	return b
}

// JSONToPoint parses canonical Point JSON.
func JSONToPoint(raw json.RawMessage) (geom.Point, error) {
	var v struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return geom.Point{}, err
	}
	return geom.Point{X: v.X, Y: v.Y}, nil
}

// RectToJSON returns the canonical JSON form of a geom.Rect.
func RectToJSON(r geom.Rect) json.RawMessage {
	b, _ := json.Marshal(struct {
		A geom.Point `json:"a"`
		B geom.Point `json:"b"`
	}{r.A, r.B})
	return b
}

// JSONToRect parses canonical Rect JSON.
func JSONToRect(raw json.RawMessage) (geom.Rect, error) {
	var v struct {
		A geom.Point `json:"a"`
		B geom.Point `json:"b"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return geom.Rect{}, err
	}
	return geom.Rect{A: v.A, B: v.B}, nil
}

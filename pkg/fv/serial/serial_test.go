package serial

import (
	"encoding/json"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

type fakeView struct {
	Title  string `json:"title"`
	Bounds geom.Rect
}

func (f *fakeView) GetTypeID() string { return "test:fake" }

func (f *fakeView) ToJSON() (json.RawMessage, error) {
	return json.Marshal(struct {
		Title  string          `json:"title"`
		Bounds json.RawMessage `json:"bounds"`
	}{f.Title, RectToJSON(f.Bounds)})
}

func (f *fakeView) FromJSON(raw json.RawMessage) error {
	var v struct {
		Title  string          `json:"title"`
		Bounds json.RawMessage `json:"bounds"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	f.Title = v.Title
	r, err := JSONToRect(v.Bounds)
	if err != nil {
		return err
	}
	f.Bounds = r
	return nil
}

func TestRegistryRoundtrip(t *testing.T) {
	Register("test:fake", func() Serializable { return &fakeView{} })
	if !CanCreate("test:fake") {
		t.Fatal("CanCreate fail")
	}
	in := &fakeView{Title: "hello", Bounds: geom.NewRect(0, 0, 80, 25)}
	raw, err := Serialize(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Deserialize(raw)
	if err != nil {
		t.Fatal(err)
	}
	gv, ok := got.(*fakeView)
	if !ok {
		t.Fatalf("type: %T", got)
	}
	if gv.Title != in.Title || !gv.Bounds.Equals(in.Bounds) {
		t.Fatalf("mismatch: got %+v", gv)
	}
}

func TestUnknownType(t *testing.T) {
	if _, err := Deserialize([]byte(`{"typeId":"never","data":{}}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestPointRect(t *testing.T) {
	p := geom.Point{X: 3, Y: 5}
	got, err := JSONToPoint(PointToJSON(p))
	if err != nil || got != p {
		t.Fatalf("Point round-trip: got %v err=%v", got, err)
	}
	r := geom.NewRect(1, 2, 10, 20)
	gr, err := JSONToRect(RectToJSON(r))
	if err != nil || !gr.Equals(r) {
		t.Fatalf("Rect round-trip: got %v err=%v", gr, err)
	}
}

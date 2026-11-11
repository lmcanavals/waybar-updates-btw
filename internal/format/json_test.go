package format_test

import (
	"encoding/json"
	"testing"

	"github.com/lmcanavals/waybar-updates-btw/internal/format"
	"github.com/lmcanavals/waybar-updates-btw/internal/protocol"
)

func TestParseUpdate(t *testing.T) {
	u, ok := format.ParseUpdate("aur/yay 13.0.1-1 -> 13.0.2-1")
	if !ok {
		t.Fatal("expected aur line to parse")
	}
	want := format.Update{Name: "yay", Source: "aur", Old: "13.0.1-1", Unchanged: "13.0.", Changed: "2-1", Level: format.LevelPatch}
	if u != want {
		t.Errorf("got %+v, want %+v", u, want)
	}

	u, ok = format.ParseUpdate("cups 2:2.4.19-1 -> 2:2.4.20-1")
	if !ok || u.Name != "cups" || u.Source != "pacman" {
		t.Errorf("pacman line parsed as %+v (ok=%v)", u, ok)
	}

	for _, bad := range []string{"", "pkg", "pkg 1.0 1.1", "pkg 1.0 => 1.1"} {
		if _, ok := format.ParseUpdate(bad); ok {
			t.Errorf("ParseUpdate(%q) should fail", bad)
		}
	}
}

func TestBuildJSON(t *testing.T) {
	resp := protocol.ResponseData{
		Changed:   true,
		Version:   9,
		Updates:   []string{"libx11 1.8.13-1 -> 1.8.13-2", "garbage"},
		Count:     2,
		Timestamp: "2026-10-06T11:04:36-05:00",
	}
	got, err := json.Marshal(format.BuildJSON(resp))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"changed":true,"version":9,"updates":[{"name":"libx11","source":"pacman","old":"1.8.13-1","unchanged":"1.8.13-","changed":"2","level":"pre"}],"count":2,"timestamp":"2026-10-06T11:04:36-05:00"}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// Empty update list must encode as [] rather than null.
	got, _ = json.Marshal(format.BuildJSON(protocol.ResponseData{Changed: true}))
	want = `{"changed":true,"version":0,"updates":[],"count":0,"timestamp":""}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

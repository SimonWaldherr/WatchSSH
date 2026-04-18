package main

import (
	"reflect"
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

func TestParseCSVSet(t *testing.T) {
	got := parseCSVSet(" web-01,db-01 , ,api-01 ")
	want := map[string]struct{}{
		"web-01": {},
		"db-01":  {},
		"api-01": {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCSVSet mismatch: got=%v want=%v", got, want)
	}
}

func TestFilterServers_ByNameAndTag(t *testing.T) {
	servers := []config.Server{
		{Name: "web-01", Tags: []string{"linux", "web"}},
		{Name: "db-01", Tags: []string{"linux", "database"}},
		{Name: "mac-01", Tags: []string{"darwin"}},
	}

	got := filterServers(
		servers,
		map[string]struct{}{"web-01": {}, "db-01": {}},
		map[string]struct{}{"web": {}},
	)
	if len(got) != 1 {
		t.Fatalf("filtered len=%d, want 1", len(got))
	}
	if got[0].Name != "web-01" {
		t.Fatalf("filtered[0].Name=%q, want web-01", got[0].Name)
	}
}

func TestFilterServers_ByTagOnly(t *testing.T) {
	servers := []config.Server{
		{Name: "web-01", Tags: []string{"linux", "web"}},
		{Name: "db-01", Tags: []string{"linux", "database"}},
		{Name: "mac-01", Tags: []string{"darwin"}},
	}

	got := filterServers(servers, nil, map[string]struct{}{"linux": {}})
	if len(got) != 2 {
		t.Fatalf("filtered len=%d, want 2", len(got))
	}
}

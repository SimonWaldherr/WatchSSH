package monitor

import (
	"context"
	"errors"
	"testing"
)

type toolRunner struct {
	out string
	err error
}

func (r toolRunner) Run(context.Context, string) (string, error) { return r.out, r.err }

func TestDiscoverStandardTools(t *testing.T) {
	tools := discoverStandardTools(context.Background(), toolRunner{out: "df\nps\nss\nunexpected\n"})
	if !tools["df"] || !tools["ps"] || !tools["ss"] {
		t.Fatalf("available tools = %#v", tools)
	}
	if tools["unexpected"] {
		t.Fatalf("unexpected tool was retained: %#v", tools)
	}
}

func TestDiscoverStandardToolsFailureIsNonCritical(t *testing.T) {
	if tools := discoverStandardTools(context.Background(), toolRunner{err: errors.New("shell unavailable")}); tools != nil {
		t.Fatalf("tools = %#v, want nil", tools)
	}
}

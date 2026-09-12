package progress

import "testing"

func TestParseLine(t *testing.T) {
	cases := []struct {
		line    string
		pct     int
		msg     string
		ok      bool
	}{
		{"::k3slab-progress::25::Pulling images", 25, "Pulling images", true},
		{"::k3slab-progress::0::", 0, "", true},
		{"::k3slab-progress::100::Done", 100, "Done", true},
		{"stderr: ::k3slab-progress::40::Gitea", 40, "Gitea", true},
		{"  ::k3slab-progress::10::Namespaces  ", 10, "Namespaces", true},
		{"normal log line", 0, "", false},
		{"::k3slab-progress::xx::bad", 0, "", false},
		{"::k3slab-progress::50", 0, "", false},
	}
	for _, tc := range cases {
		pct, msg, ok := ParseLine(tc.line)
		if ok != tc.ok || pct != tc.pct || msg != tc.msg {
			t.Fatalf("ParseLine(%q) = (%d,%q,%v) want (%d,%q,%v)",
				tc.line, pct, msg, ok, tc.pct, tc.msg, tc.ok)
		}
	}
}

func TestHubResetSetComplete(t *testing.T) {
	h := New()
	if snap := h.Snapshot(); snap.Active || snap.Pct != 0 {
		t.Fatalf("initial: %+v", snap)
	}

	h.Reset()
	if snap := h.Snapshot(); !snap.Active || snap.Pct != 0 || snap.Message != "" {
		t.Fatalf("after Reset: %+v", snap)
	}

	h.Set(30, "pull")
	if snap := h.Snapshot(); snap.Pct != 30 || snap.Message != "pull" || !snap.Active {
		t.Fatalf("after Set: %+v", snap)
	}

	// Non-decreasing.
	h.Set(10, "backwards")
	if snap := h.Snapshot(); snap.Pct != 30 || snap.Message != "backwards" {
		t.Fatalf("decrease ignored: %+v", snap)
	}

	h.Set(200, "clamp")
	if snap := h.Snapshot(); snap.Pct != 100 {
		t.Fatalf("clamp: %+v", snap)
	}

	h.Complete()
	if snap := h.Snapshot(); snap.Active || snap.Pct != 100 {
		t.Fatalf("after Complete: %+v", snap)
	}

	// Set ignored when inactive.
	h.Set(50, "ignored")
	if snap := h.Snapshot(); snap.Pct != 100 || snap.Message != "clamp" {
		t.Fatalf("set after complete: %+v", snap)
	}
}

func TestHubFailKeepsPct(t *testing.T) {
	h := New()
	h.Reset()
	h.Set(35, "Installing Gitea and Argo CD")
	h.Fail()
	snap := h.Snapshot()
	if snap.Active || snap.Pct != 35 || snap.Message != "Installing Gitea and Argo CD" {
		t.Fatalf("after Fail: %+v", snap)
	}
}

func TestHubBroadcast(t *testing.T) {
	h := New()
	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	h.Reset()
	select {
	case snap := <-ch:
		if !snap.Active || snap.Pct != 0 {
			t.Fatalf("reset event: %+v", snap)
		}
	default:
		t.Fatal("expected Reset broadcast")
	}

	h.Set(55, "mid")
	select {
	case snap := <-ch:
		if snap.Pct != 55 || snap.Message != "mid" {
			t.Fatalf("set event: %+v", snap)
		}
	default:
		t.Fatal("expected Set broadcast")
	}
}

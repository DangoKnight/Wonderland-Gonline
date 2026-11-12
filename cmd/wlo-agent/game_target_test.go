package main

import "testing"

func TestAgentGameTargetStaysInConfiguredFamily(t *testing.T) {
	for _, tc := range []struct{ request, want string }{
		{"", "127.0.0.1:6714"}, {"other.example:6414", "127.0.0.1:6714"},
		{"other.example:6415", "127.0.0.1:6715"}, {"other.example:6416", "127.0.0.1:6716"},
		{"127.0.0.1:6715", "127.0.0.1:6715"},
	} {
		got, err := agentGameTarget(tc.request, "127.0.0.1:6714")
		if err != nil || got != tc.want {
			t.Fatalf("%s: %s %v", tc.request, got, err)
		}
	}
	for _, request := range []string{"localhost:8080", "localhost:6417", "invalid"} {
		if _, err := agentGameTarget(request, "127.0.0.1:6714"); err == nil {
			t.Fatalf("accepted %s", request)
		}
	}
}

func TestObservationPointerDataOnly(t *testing.T) {
	value := map[string]any{"a/b": map[string]any{"~key": []any{"first", true}}}
	got, ok := observationValue(value, "/a~1b/~0key/1")
	if !ok || got != true {
		t.Fatal(got, ok)
	}
	for _, p := range []string{"a/b", "/a~1b/~0key/-1", "/a~1b/~0key/01", "/a~1b/~0key/2", "/a~1b/missing"} {
		if _, ok := observationValue(value, p); ok {
			t.Fatal(p)
		}
	}
}

func TestDiagnosticInputDisabledByDefault(t *testing.T) {
	m := &Manager{}
	for _, method := range []string{"agent.command", "agent.eval"} {
		r := dispatch(m, Request{Method: method})
		if r.Error == nil {
			t.Fatalf("%s enabled without opt-in", method)
		}
	}
}

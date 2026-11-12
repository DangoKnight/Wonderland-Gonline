package main

import "testing"

func TestCommandResponseRejectsClientFailures(t *testing.T) {
	if err := commandResponse("ok"); err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{"err: action rejected", "no wlo", "", "unexpected"} {
		if commandResponse(response) == nil {
			t.Fatalf("reported success for %q", response)
		}
	}
}

func TestStructuredResponsePropagatesAdmissionFailure(t *testing.T) {
	for _, raw := range []string{`{"error":"unknown command"}`, `{"error":"session unavailable"}`, `undefined`} {
		if _, err := structuredResponse([]byte(raw)); err == nil {
			t.Fatalf("false success: %s", raw)
		}
	}
	for _, raw := range []string{`false`, `[]`, `{"id":"ticket","status":"rejected","error":"insufficient SP"}`, `{"id":"ticket","status":"accepted"}`} {
		if _, err := structuredResponse([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

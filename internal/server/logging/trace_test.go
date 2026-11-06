package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLuckyDrawTraceDecodedCatalogAndResult(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", TraceAttrs([]byte{104, 1, 7}, false)...)
	logger.Debug("result", TraceAttrs([]byte{104, 1, 2, 8, 1}, true)...)
	logger.Debug("catalog", TraceAttrs([]byte{104, 1, 1, 2, 1, 176, 125, 2}, true)...)
	for _, want := range []string{`"payload_hex":"680107"`, `"lucky_draw_intent":"spin"`, `"expected_request_bytes":2`, `"payload_hex":"6801020801"`, `"lucky_draw_used":2`, `"reward_count":1`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal("missing diagnostic", want, logs.String())
		}
	}
}

func TestLuckyDrawTraceKeepsSensitivePacketsPrivateAndBoundsMalformedDraws(t *testing.T) {
	for _, sent := range []bool{false, true} {
		for _, prefix := range [][]byte{{63, 4}, {9, 1}, {35, 1}, {35, 12}, {2, 1}, {14, 1}} {
			attrs := TraceAttrs(append(prefix, []byte("private-password-or-message")...), sent)
			for i := 0; i < len(attrs); i += 2 {
				if attrs[i] == "payload_hex" {
					t.Fatal("sensitive packet exposed", prefix, sent)
				}
			}
		}
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	payload := append([]byte{104, 99}, bytes.Repeat([]byte{255}, 100)...)
	logger.Debug("request", TraceAttrs(payload, false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded malformed trace", logs.String())
	}
}

func TestNativePoseTraceShowsOwnerAndExpression(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", TraceAttrs([]byte{32, 1, 9}, false)...)
	logger.Debug("reply", TraceAttrs([]byte{32, 1, 17, 39, 0, 0, 9}, true)...)
	for _, want := range []string{`"payload_hex":"200109"`, `"payload_hex":"20011127000009"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal("missing expression trace", logs.String())
		}
	}
	logger.Debug("malformed", TraceAttrs(append([]byte{32, 1}, bytes.Repeat([]byte{255}, 100)...), false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded expression trace", logs.String())
	}
}

func TestNativeMovementTraceBoundsPayload(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("movement", TraceAttrs([]byte{6, 2, 255, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, false)...)
	if !strings.Contains(logs.String(), `"payload_hex":"0602ff0102030405060708090a0b0c"`) {
		t.Fatal("missing AC6:2 trace", logs.String())
	}
	logger.Debug("oversized", TraceAttrs(append([]byte{6, 2}, bytes.Repeat([]byte{255}, 100)...), false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded movement trace", logs.String())
	}
}

func TestPotentialPillTraceBoundsPayload(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", TraceAttrs(append([]byte{23, 126, 50, 2}, bytes.Repeat([]byte{255}, 100)...), false)...)
	logger.Debug("reply", TraceAttrs([]byte{23, 213, 1, 2, 12}, true)...)
	for _, want := range []string{`"payload_hex":"177e3202"`, `"payload_truncated":true`, `"payload_hex":"17d501020c"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal(want, logs.String())
		}
	}
	if strings.Contains(logs.String(), "ffffffff") {
		t.Fatal("unbounded potential trace")
	}
}

func TestNativeCompoundPacketTrace(t *testing.T) {
	attrs := TraceAttrs([]byte{23, 14, 2, 1, 2}, false)
	found := false
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == "payload_hex" {
			found = true
			if attrs[i+1] != "170e020102" {
				t.Fatal(attrs)
			}
		}
	}
	if !found {
		t.Fatal("compound request bytes not logged")
	}
}

func TestSpeechTraceDecodesFullTalkReference(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("speech", TraceAttrs([]byte{20, 1, 0, 0, 0, 2, 1, 3, 4, 0, 1, 0, 0, 0, 0, 0x56, 0x34, 0x12}, true)...)
	if !strings.Contains(logs.String(), `"talk_id":1193046`) || !strings.Contains(logs.String(), `"event_actor":4`) {
		t.Fatal(logs.String())
	}
}

package headless

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const maximumObservationWait = 30 * time.Second
const observationPoll = 100 * time.Millisecond

func observationValue(value any, pointer string) (any, bool) {
	if pointer == "" {
		return value, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || part[i+1] != '0' && part[i+1] != '1' {
					return nil, false
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[part]
			if !ok {
				return nil, false
			}
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) || strconv.Itoa(i) != part {
				return nil, false
			}
			value = v[i]
		default:
			return nil, false
		}
	}
	return value, true
}
func (h *host) observe(parent context.Context, params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		ID        string          `json:"id"`
		Session   int             `json:"session"`
		Pointer   string          `json:"pointer"`
		Equals    json.RawMessage `json:"equals"`
		TimeoutMS int             `json:"timeoutMs"`
	}
	if json.Unmarshal(params, &p) != nil || p.Session < 0 || len(p.Pointer) > 1024 || p.TimeoutMS < 1 || p.TimeoutMS > int(maximumObservationWait/time.Millisecond) || !json.Valid(p.Equals) {
		return nil, errors.New("invalid observation")
	}
	var expected any
	_ = json.Unmarshal(p.Equals, &expected)
	ctx, cancel := context.WithTimeout(parent, time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()
	query, _ := json.Marshal(map[string]any{"id": p.ID, "session": p.Session})
	tick := time.NewTicker(observationPoll)
	defer tick.Stop()
	for {
		data, err := h.do(ctx, func() (any, error) { return h.dispatch(Request{JSONRPC: "2.0", Method: "agent.state", Params: query}) })
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return nil, err
		}
		var state any
		if json.Unmarshal(data, &state) != nil {
			return nil, errors.New("invalid state")
		}
		if value, ok := observationValue(state, p.Pointer); ok && reflect.DeepEqual(value, expected) {
			return json.Marshal(map[string]any{"status": "observed", "value": value, "serverConfirmed": false})
		}
		select {
		case <-ctx.Done():
			return json.Marshal(map[string]any{"status": "timed_out", "serverConfirmed": false})
		case <-tick.C:
		}
	}
	return json.Marshal(map[string]any{"status": "timed_out", "serverConfirmed": false})
}

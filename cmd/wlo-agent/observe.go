package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const observerMaximumWait = 30 * time.Second
const observerPollInterval = 100 * time.Millisecond

type observationRequest struct {
	ID        string          `json:"id"`
	Session   int             `json:"session"`
	Pointer   string          `json:"pointer"`
	Equals    json.RawMessage `json:"equals"`
	TimeoutMS int             `json:"timeoutMs"`
}

// JSON pointers address data only. No expressions, scripts or implicit actions.
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
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
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
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(v) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = v[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func (m *Manager) Observe(p observationRequest) (any, error) {
	if p.ID == "" || p.Session < 0 || len(p.Pointer) > 1024 || p.Pointer != "" && !strings.HasPrefix(p.Pointer, "/") || p.TimeoutMS < 1 || p.TimeoutMS > int(observerMaximumWait/time.Millisecond) || !json.Valid(p.Equals) {
		return nil, fmt.Errorf("invalid observation parameters")
	}
	a, ok := m.Get(p.ID)
	if !ok {
		return nil, fmt.Errorf("agent not found")
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Duration(p.TimeoutMS)*time.Millisecond)
	defer cancel()
	var expected any
	if json.Unmarshal(p.Equals, &expected) != nil {
		return nil, fmt.Errorf("invalid expected value")
	}
	argument, _ := json.Marshal(p.Session)
	ticker := time.NewTicker(observerPollInterval)
	defer ticker.Stop()
	for {
		raw, err := m.structuredAPI(ctx, p.ID, "state", argument)
		if err != nil {
			if ctx.Err() != nil {
				return map[string]any{"status": "timed_out", "serverConfirmed": false}, nil
			}
			return nil, err
		}
		var state any
		if json.Unmarshal(raw, &state) != nil {
			return nil, fmt.Errorf("invalid state")
		}
		if value, found := observationValue(state, p.Pointer); found && reflect.DeepEqual(value, expected) {
			return map[string]any{"status": "observed", "value": value, "serverConfirmed": false}, nil
		}
		select {
		case <-ctx.Done():
			return map[string]any{"status": "timed_out", "serverConfirmed": false}, nil
		case <-ticker.C:
		}
	}
}

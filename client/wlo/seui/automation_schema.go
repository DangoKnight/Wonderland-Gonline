package seui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Payload schemas are compiled with their owning forms. The registry is filled
// at package initialization and immutable while any client is running.
var automationPayloads = map[string]map[string]any{}

func RegisterAutomation(kind string, payloads map[string]any) {
	if _, exists := automationPayloads[kind]; exists {
		panic("duplicate automation kind: " + kind)
	}
	automationPayloads[kind] = payloads
}

type AutomationSchema struct {
	Nullable   bool                        `json:"nullable,omitempty"`
	Type       string                      `json:"type"`
	Properties map[string]AutomationSchema `json:"properties,omitempty"`
	Required   []string                    `json:"required,omitempty"`
	Items      *AutomationSchema           `json:"items,omitempty"`
	Length     *int                        `json:"length,omitempty"`
}

func SchemaOf(sample any) AutomationSchema {
	if sample == nil {
		return AutomationSchema{Type: "null"}
	}
	return schemaFor(reflect.TypeOf(sample))
}
func schemaFor(t reflect.Type) AutomationSchema {
	switch t.Kind() {
	case reflect.Bool:
		return AutomationSchema{Type: "boolean"}
	case reflect.String:
		return AutomationSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return AutomationSchema{Type: "integer"}
	case reflect.Array, reflect.Slice:
		item := schemaFor(t.Elem())
		s := AutomationSchema{Type: "array", Items: &item, Nullable: t.Kind() == reflect.Slice}
		if t.Kind() == reflect.Array {
			n := t.Len()
			s.Length = &n
		}
		return s
	case reflect.Struct:
		s := AutomationSchema{Type: "object", Properties: map[string]AutomationSchema{}, Required: []string{}}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, options, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			s.Properties[name] = schemaFor(f.Type)
			if !strings.Contains(options, "omitempty") {
				s.Required = append(s.Required, name)
			}
		}
		return s
	case reflect.Pointer:
		return schemaFor(t.Elem())
	default:
		return AutomationSchema{Type: "any"}
	}
}
func AutomationPayloadSchemas(kind string) map[string]AutomationSchema {
	out := map[string]AutomationSchema{}
	for action, sample := range automationPayloads[kind] {
		out[action] = SchemaOf(sample)
	}
	return out
}

func AutomationContract() map[string]map[string]AutomationSchema {
	out := map[string]map[string]AutomationSchema{}
	for kind := range automationPayloads {
		out[kind] = AutomationPayloadSchemas(kind)
	}
	return out
}
func ValidateAutomationSchema(raw json.RawMessage, schema AutomationSchema) error {
	return schemaShape(raw, schema)
}
func AutomationDescription(info AutomationInfo) AutomationInfo {
	info.Payloads = AutomationPayloadSchemas(info.Kind)
	return info
}
func ValidateAutomationAction(info AutomationInfo, action string, raw json.RawMessage) error {
	return validateAutomationPayload(info, action, raw)
}
func schemaShape(raw json.RawMessage, schema AutomationSchema) error {
	if schema.Type == "any" {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		if schema.Type == "null" || schema.Nullable {
			return nil
		}
		return fmt.Errorf("null is not valid for this action")
	}
	switch schema.Type {
	case "null":
		return fmt.Errorf("action takes a null value")
	case "object":
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fmt.Errorf("object required")
		}
		normalized := map[string]json.RawMessage{}
		for key, value := range values {
			var canonical string
			for name := range schema.Properties {
				if strings.EqualFold(name, key) {
					canonical = name
					break
				}
			}
			if canonical == "" {
				return fmt.Errorf("unknown action parameter")
			}
			if _, exists := normalized[canonical]; exists {
				return fmt.Errorf("duplicate action parameter")
			}
			normalized[canonical] = value
		}
		for _, name := range schema.Required {
			if _, ok := normalized[name]; !ok {
				return fmt.Errorf("missing action parameter: %s", name)
			}
		}
		for name, value := range normalized {
			if err := schemaShape(value, schema.Properties[name]); err != nil {
				return err
			}
		}
	case "array":
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fmt.Errorf("array required")
		}
		if schema.Length != nil && len(values) != *schema.Length {
			return fmt.Errorf("incorrect array length")
		}
		for _, value := range values {
			if err := schemaShape(value, *schema.Items); err != nil {
				return err
			}
		}
	case "integer":
		var n json.Number
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var value any
		if d.Decode(&value) != nil {
			return fmt.Errorf("integer required")
		}
		var ok bool
		n, ok = value.(json.Number)
		if !ok {
			return fmt.Errorf("integer required")
		}
		if _, err := n.Int64(); err != nil {
			return fmt.Errorf("integer required")
		}
	case "string":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return fmt.Errorf("string required")
		}
	case "boolean":
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return fmt.Errorf("boolean required")
		}
	}
	return nil
}

func validateAutomationPayload(info AutomationInfo, action string, raw json.RawMessage) error {
	if schemas := AutomationPayloadSchemas(info.Kind); len(schemas) > 0 {
		schema, ok := schemas[action]
		if !ok {
			return fmt.Errorf("action schema unavailable")
		}
		return schemaShape(raw, schema)
	}
	var sample any
	switch action {
	case "set":
		sample = ""
	case "select", "open", "remove":
		if info.Kind == "list" || info.Kind == "scroll" {
			sample = 0
		}
	}
	return schemaShape(raw, SchemaOf(sample))
}

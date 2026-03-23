package flex

import (
	"encoding/json"
	"fmt"
)

// Config wraps raw plugin configuration JSON for type-safe access.
type Config struct {
	Raw json.RawMessage
}

// ConfigSchema is a JSON Schema document describing the expected plugin configuration.
type ConfigSchema json.RawMessage

// ValidateConfig validates raw configuration JSON against a JSON Schema.
// It performs lightweight validation: required fields, type checking for
// string, integer, number, boolean, array, and object types.
// No $ref resolution -- plugins should use flat schemas.
func ValidateConfig(schema ConfigSchema, raw json.RawMessage) error {
	var schemaDef schemaDefinition
	if err := json.Unmarshal(schema, &schemaDef); err != nil {
		return fmt.Errorf("parsing config schema: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing config JSON: %w", err)
	}

	// Check required fields
	for _, field := range schemaDef.Required {
		if _, ok := data[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Check property types
	for name, prop := range schemaDef.Properties {
		val, ok := data[name]
		if !ok {
			continue
		}
		if err := validateType(name, prop.Type, val); err != nil {
			return err
		}
	}

	return nil
}

type schemaDefinition struct {
	Type       string                      `json:"type"`
	Required   []string                    `json:"required"`
	Properties map[string]propertyDefinition `json:"properties"`
}

type propertyDefinition struct {
	Type string `json:"type"`
}

func validateType(name, expectedType string, value interface{}) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %s: expected string, got %T", name, value)
		}
	case "integer":
		f, ok := value.(float64)
		if !ok {
			return fmt.Errorf("field %s: expected integer, got %T", name, value)
		}
		if f != float64(int64(f)) {
			return fmt.Errorf("field %s: expected integer, got float", name)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("field %s: expected number, got %T", name, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %s: expected boolean, got %T", name, value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field %s: expected array, got %T", name, value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("field %s: expected object, got %T", name, value)
		}
	default:
		// Unknown type -- skip validation
	}
	return nil
}


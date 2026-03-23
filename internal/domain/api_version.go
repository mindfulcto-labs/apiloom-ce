package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// semverRegexp validates version tags in the format vMAJOR.MINOR.PATCH.
var semverRegexp = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// APIVersion records a snapshot of a route configuration at a specific version tag.
type APIVersion struct {
	ID         uuid.UUID       `json:"id"`
	RouteID    uuid.UUID       `json:"route_id"`
	VersionTag string          `json:"version_tag"`
	Config     json.RawMessage `json:"config"`
	Changelog  string          `json:"changelog"`
	IsBreaking bool            `json:"is_breaking"`
	CreatedAt  time.Time       `json:"created_at"`
	CreatedBy  string          `json:"created_by"`
}

// Validate checks that the APIVersion fields satisfy business rules.
func (v *APIVersion) Validate() error {
	if v.RouteID == uuid.Nil {
		return fmt.Errorf("route_id is required: %w", ErrInvalidConfig)
	}
	v.VersionTag = strings.TrimSpace(v.VersionTag)
	if !semverRegexp.MatchString(v.VersionTag) {
		return fmt.Errorf("version_tag must be semver (vMAJOR.MINOR.PATCH): %w", ErrInvalidConfig)
	}
	return nil
}

// ConfigDiff describes the structural difference between two JSON configurations.
type ConfigDiff struct {
	Added   []string      `json:"added"`
	Removed []string      `json:"removed"`
	Changed []FieldChange `json:"changed"`
}

// FieldChange records a single field that changed between two configurations.
type FieldChange struct {
	Field    string `json:"field"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// CanaryConfig holds the active canary routing configuration for a route.
type CanaryConfig struct {
	RouteID          uuid.UUID `json:"route_id"`
	PrimaryVersionID uuid.UUID `json:"primary_version_id"`
	CanaryVersionID  uuid.UUID `json:"canary_version_id"`
	CanaryWeight     int       `json:"canary_weight"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Validate checks that the CanaryConfig fields satisfy business rules.
func (c *CanaryConfig) Validate() error {
	if c.RouteID == uuid.Nil {
		return fmt.Errorf("route_id is required: %w", ErrInvalidConfig)
	}
	if c.PrimaryVersionID == uuid.Nil {
		return fmt.Errorf("primary_version_id is required: %w", ErrInvalidConfig)
	}
	if c.CanaryVersionID == uuid.Nil {
		return fmt.Errorf("canary_version_id is required: %w", ErrInvalidConfig)
	}
	if c.PrimaryVersionID == c.CanaryVersionID {
		return fmt.Errorf("primary and canary versions must differ: %w", ErrInvalidConfig)
	}
	if c.CanaryWeight < 0 || c.CanaryWeight > 100 {
		return fmt.Errorf("canary_weight must be 0-100: %w", ErrInvalidConfig)
	}
	return nil
}

// DiffConfigs compares two JSON configurations and returns a structured diff.
func DiffConfigs(oldConfig, newConfig json.RawMessage) (ConfigDiff, error) {
	var oldMap, newMap map[string]interface{}
	if err := json.Unmarshal(oldConfig, &oldMap); err != nil {
		return ConfigDiff{}, fmt.Errorf("unmarshalling old config: %w", err)
	}
	if err := json.Unmarshal(newConfig, &newMap); err != nil {
		return ConfigDiff{}, fmt.Errorf("unmarshalling new config: %w", err)
	}

	diff := ConfigDiff{
		Added:   []string{},
		Removed: []string{},
		Changed: []FieldChange{},
	}

	// Find removed and changed fields.
	diffMaps("", oldMap, newMap, &diff)

	// Find added fields.
	findAdded("", oldMap, newMap, &diff)

	return diff, nil
}

// diffMaps recursively compares oldMap against newMap, recording removed and changed fields.
func diffMaps(prefix string, oldMap, newMap map[string]interface{}, diff *ConfigDiff) {
	for key, oldVal := range oldMap {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		newVal, exists := newMap[key]
		if !exists {
			diff.Removed = append(diff.Removed, fullKey)
			continue
		}

		// Both are maps — recurse.
		oldNested, oldIsMap := oldVal.(map[string]interface{})
		newNested, newIsMap := newVal.(map[string]interface{})
		if oldIsMap && newIsMap {
			diffMaps(fullKey, oldNested, newNested, diff)
			continue
		}

		// Compare serialized values.
		oldStr := toJSONString(oldVal)
		newStr := toJSONString(newVal)
		if oldStr != newStr {
			diff.Changed = append(diff.Changed, FieldChange{
				Field:    fullKey,
				OldValue: oldStr,
				NewValue: newStr,
			})
		}
	}
}

// findAdded recursively finds keys in newMap not present in oldMap.
func findAdded(prefix string, oldMap, newMap map[string]interface{}, diff *ConfigDiff) {
	for key, newVal := range newMap {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		oldVal, exists := oldMap[key]
		if !exists {
			diff.Added = append(diff.Added, fullKey)
			continue
		}

		// Both are maps — recurse.
		oldNested, oldIsMap := oldVal.(map[string]interface{})
		newNested, newIsMap := newVal.(map[string]interface{})
		if oldIsMap && newIsMap {
			findAdded(fullKey, oldNested, newNested, diff)
		}
	}
}

// toJSONString serializes a value to its JSON string representation for comparison.
func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// IsBreakingChange returns true if the diff contains breaking changes.
// Breaking changes: removed fields, changed field types (value type changed).
func IsBreakingChange(diff ConfigDiff) bool {
	if len(diff.Removed) > 0 {
		return true
	}
	// A changed field where the JSON type changed is breaking.
	for _, c := range diff.Changed {
		if typeChanged(c.OldValue, c.NewValue) {
			return true
		}
	}
	return false
}

// typeChanged returns true if the JSON type of two serialized values differs.
func typeChanged(oldVal, newVal string) bool {
	return jsonType(oldVal) != jsonType(newVal)
}

// jsonType returns the JSON type of a serialized value.
func jsonType(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "empty"
	}
	switch s[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// GenerateMigrationNotes produces human-readable migration notes from a config diff.
func GenerateMigrationNotes(diff ConfigDiff) []string {
	var notes []string
	for _, field := range diff.Removed {
		notes = append(notes, fmt.Sprintf("Field '%s' removed (BREAKING)", field))
	}
	for _, field := range diff.Added {
		notes = append(notes, fmt.Sprintf("New field '%s' added", field))
	}
	for _, c := range diff.Changed {
		label := "modified"
		if typeChanged(c.OldValue, c.NewValue) {
			label = "type changed (BREAKING)"
		}
		notes = append(notes, fmt.Sprintf("Field '%s' %s: %s -> %s", c.Field, label, c.OldValue, c.NewValue))
	}
	return notes
}

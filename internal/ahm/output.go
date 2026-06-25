package ahm

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
)

func (a *app) emit(value any) error {
	switch {
	case a.opts.json:
		return a.emitJSON(value)
	case a.opts.plain:
		return a.emitCompactJSON(value)
	default:
		return a.emitText(value)
	}
}

func (a *app) emitJSON(value any) error {
	if value == nil {
		_, err := fmt.Fprintln(a.out, "null")
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.out, string(data))
	return err
}

func (a *app) emitCompactJSON(value any) error {
	if value == nil {
		_, err := fmt.Fprintln(a.out, "{}")
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.out, string(data))
	return err
}

func (a *app) emitText(value any) error {
	if value == nil {
		_, err := fmt.Fprintln(a.out, "none")
		return err
	}
	return writeTextValue(a.out, value, "")
}

// writeTextValue writes a human-friendly text representation of value to w.
// prefix is indentation for the current level.
func writeTextValue(w io.Writer, value any, prefix string) error {
	if value == nil {
		_, err := fmt.Fprintln(w, "none")
		return err
	}

	switch v := value.(type) {
	case string:
		_, err := fmt.Fprintln(w, v)
		return err
	case bool:
		_, err := fmt.Fprintf(w, "%v\n", v)
		return err
	case int:
		_, err := fmt.Fprintf(w, "%d\n", v)
		return err
	case int64:
		_, err := fmt.Fprintf(w, "%d\n", v)
		return err
	case float64:
		_, err := fmt.Fprintf(w, "%v\n", v)
		return err
	case map[string]any:
		return writeMapText(w, v, prefix)
	case map[string]string:
		return writeStringMapText(w, v, prefix)
	case map[string][]string:
		return writeStringSliceMapText(w, v, prefix)
	case map[string]int:
		return writeIntMapText(w, v, prefix)
	case []any:
		return writeSliceText(w, v, prefix)
	case []map[string]any:
		return writeMapSliceText(w, v, prefix)
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice:
			return writeReflectSliceText(w, rv, prefix)
		case reflect.Struct, reflect.Pointer:
			return writeFallbackJSON(w, value, prefix)
		default:
			_, err := fmt.Fprintf(w, "%v\n", value)
			return err
		}
	}
}

// writeMapText writes a map[string]any with each key on its own line.
// For simple values: `key: value\n`
// For complex values (maps, slices): `key:\n  <nested content>`
func writeMapText(w io.Writer, m map[string]any, prefix string) error {
	keys := sortedStringKeys(m)
	for _, key := range keys {
		val := m[key]
		if isComplexVal(val) {
			// Complex value: print "key:" on its own line, then indented value.
			if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
				return err
			}
			if err := writeTextValue(w, val, prefix+"  "); err != nil {
				return err
			}
		} else {
			// Simple value: print "key: value" on one line.
			if _, err := fmt.Fprintf(w, "%s%s: ", prefix, key); err != nil {
				return err
			}
			if err := writeSimpleValue(w, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeSimpleValue writes a simple scalar value (string, bool, number, or nil).
func writeSimpleValue(w io.Writer, value any) error {
	if value == nil {
		_, err := fmt.Fprintln(w, "none")
		return err
	}
	switch v := value.(type) {
	case string:
		_, err := fmt.Fprintln(w, v)
		return err
	case bool:
		_, err := fmt.Fprintf(w, "%v\n", v)
		return err
	case int:
		_, err := fmt.Fprintf(w, "%d\n", v)
		return err
	case int64:
		_, err := fmt.Fprintf(w, "%d\n", v)
		return err
	case float64:
		_, err := fmt.Fprintf(w, "%v\n", v)
		return err
	default:
		_, err := fmt.Fprintf(w, "%v\n", value)
		return err
	}
}

func writeSliceText(w io.Writer, slice []any, prefix string) error {
	if len(slice) == 0 {
		_, err := fmt.Fprintln(w, prefix+"[]")
		return err
	}
	for _, item := range slice {
		if _, err := fmt.Fprintf(w, "%s- ", prefix); err != nil {
			return err
		}
		if err := writeTextValue(w, item, prefix+"  "); err != nil {
			return err
		}
	}
	return nil
}

func writeMapSliceText(w io.Writer, maps []map[string]any, prefix string) error {
	if len(maps) == 0 {
		_, err := fmt.Fprintln(w, prefix+"[]")
		return err
	}
	for _, m := range maps {
		if _, err := fmt.Fprintf(w, "%s- ", prefix); err != nil {
			return err
		}
		if err := writeMapTextInline(w, m, prefix+"  "); err != nil {
			return err
		}
	}
	return nil
}

// writeMapTextInline writes a map as a list item. The first field is on the
// same line as the `- ` marker; subsequent fields are indented.
func writeMapTextInline(w io.Writer, m map[string]any, prefix string) error {
	keys := sortedStringKeys(m)
	for i, key := range keys {
		val := m[key]
		if i == 0 {
			// First field goes after the `- ` marker.
			if isComplexVal(val) {
				// Complex: newline after key.
				if _, err := fmt.Fprintf(w, "%s:\n", key); err != nil {
					return err
				}
				if err := writeTextValue(w, val, prefix+"  "); err != nil {
					return err
				}
			} else {
				// Simple: key: value on one line.
				if _, err := fmt.Fprintf(w, "%s: ", key); err != nil {
					return err
				}
				if err := writeSimpleValue(w, val); err != nil {
					return err
				}
			}
		} else {
			// Subsequent fields: indented.
			if isComplexVal(val) {
				if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
					return err
				}
				if err := writeTextValue(w, val, prefix+"  "); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s%s: ", prefix, key); err != nil {
					return err
				}
				if err := writeSimpleValue(w, val); err != nil {
					return err
				}
			}
		}
		// No blank line between items in a slice.
	}
	return nil
}

func writeStringMapText(w io.Writer, m map[string]string, prefix string) error {
	keys := sortedStringKeys(m)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s%s: %s\n", prefix, key, m[key]); err != nil {
			return err
		}
	}
	return nil
}

func writeStringSliceMapText(w io.Writer, m map[string][]string, prefix string) error {
	keys := sortedStringKeys(m)
	for _, key := range keys {
		items := m[key]
		if len(items) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, key); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(w, "%s  %s\n", prefix, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeIntMapText(w io.Writer, m map[string]int, prefix string) error {
	keys := sortedStringKeys(m)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s%s: %d\n", prefix, key, m[key]); err != nil {
			return err
		}
	}
	return nil
}

func writeReflectSliceText(w io.Writer, rv reflect.Value, prefix string) error {
	if rv.Len() == 0 {
		_, err := fmt.Fprintln(w, prefix+"[]")
		return err
	}
	for i := range rv.Len() {
		elem := rv.Index(i).Interface()
		if _, err := fmt.Fprintf(w, "%s- ", prefix); err != nil {
			return err
		}
		if err := writeTextValue(w, elem, prefix+"  "); err != nil {
			return err
		}
	}
	return nil
}

func writeFallbackJSON(w io.Writer, value any, prefix string) error {
	data, err := json.MarshalIndent(value, prefix, "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isComplexVal(value any) bool {
	switch value.(type) {
	case map[string]any, []any, []map[string]any:
		return true
	}
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	kind := rv.Kind()
	return kind == reflect.Map || kind == reflect.Slice || kind == reflect.Struct || kind == reflect.Pointer
}

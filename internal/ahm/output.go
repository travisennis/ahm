package ahm

import (
	"encoding/json"
	"fmt"
)

func (a *app) emit(value any) error {
	if a.opts.json {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	if m, ok := value.(map[string][]string); ok {
		for _, key := range []string{"created", "updated", "skipped", "conflicts", "directories", "metadata", "indexes"} {
			items := m[key]
			if len(items) == 0 {
				continue
			}
			fmt.Fprintf(a.out, "%s:\n", key)
			for _, item := range items {
				fmt.Fprintf(a.out, "  %s\n", item)
			}
		}
		return nil
	}
	if a.opts.plain {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.out, string(data))
	return err
}

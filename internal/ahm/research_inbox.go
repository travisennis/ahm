package ahm

import (
	"os"
	"strings"
	"time"
)

const defaultResearchInboxStaleDays = 21

func (m metadata) researchInboxStaleThreshold() (int, bool) {
	if m.Research == nil || m.Research.InboxStaleDays == nil {
		return defaultResearchInboxStaleDays, true
	}
	if *m.Research.InboxStaleDays == 0 {
		return 0, false
	}
	return *m.Research.InboxStaleDays, true
}

func researchNoteAgeDays(path string, now time.Time) (int, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return 0, err
	}
	noteTime, ok := researchNoteDate(data)
	if !ok {
		info, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		noteTime = info.ModTime()
	}
	days := int(now.UTC().Sub(noteTime.UTC()) / (24 * time.Hour))
	if days < 0 {
		return 0, nil
	}
	return days, nil
}

func researchNoteDate(data []byte) (time.Time, bool) {
	values := map[string]string{}
	meta, body, err := parseFrontMatter(string(data))
	if err == nil {
		for key, value := range meta {
			values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	} else {
		body = string(data)
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "updated" && key != "date" && key != "created" {
			continue
		}
		if _, exists := values[key]; !exists {
			values[key] = strings.TrimSpace(value)
		}
	}

	for _, key := range []string{"updated", "date", "created"} {
		if parsed, ok := parseResearchDate(values[key]); ok {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseResearchDate(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		parsed, err := time.Parse(layout, strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

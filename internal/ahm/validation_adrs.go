package ahm

import (
	"fmt"
	"sort"
	"strings"
)

func validateADRs(root string, report *validationReport) {
	adrs, _ := report.cache.adrList(root)
	byID := map[string][]ADR{}
	for _, adr := range adrs {
		if adr.ID != "" {
			byID[adr.ID] = append(byID[adr.ID], adr)
		}
	}
	for id, matches := range byID {
		if len(matches) < 2 {
			continue
		}
		paths := make([]string, 0, len(matches))
		for _, adr := range matches {
			paths = append(paths, relPath(root, adr.Path))
		}
		sort.Strings(paths)
		report.addError("adr_duplicate_id", "", fmt.Sprintf("ADR ID %s is used by multiple files: %s", id, strings.Join(paths, ", ")))
	}

	for _, adr := range adrs {
		rel := relPath(root, adr.Path)
		switch adr.Kind {
		case adrKindMalformed:
			if strings.Contains(adr.ParseError, "does not match filename id") {
				report.addError("adr_id_mismatch", rel, adr.ParseError)
			} else {
				report.addError("adr_malformed", rel, adr.ParseError)
			}
			continue
		case adrKindLegacy:
			report.addWarning("adr_legacy_format", rel, "legacy ADR format; convert it to MADR front matter manually")
			continue
		}

		status := strings.TrimSpace(adr.Status)
		if !validADRStatus(status) {
			report.addError("adr_invalid_status", rel, fmt.Sprintf("unsupported ADR status %q", adr.Status))
			continue
		}
		replacement, ok := strings.CutPrefix(status, "superseded by ADR-")
		if ok && len(byID[replacement]) == 0 {
			report.addError("adr_supersede_missing", rel, fmt.Sprintf("ADR %s is superseded by missing ADR-%s", adr.ID, replacement))
		}
	}
}

package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	adrKindMADR      = "madr"
	adrKindLegacy    = "legacy"
	adrKindMalformed = "malformed"
)

var adrSupersededStatusPattern = regexp.MustCompile(`^superseded by ADR-[0-9]{3}$`)

// ADR is the parsed representation of an Architecture Decision Record file.
type ADR struct {
	ID             string
	Slug           string
	Title          string
	Status         string
	Date           string
	DecisionMakers string
	Consulted      string
	Informed       string
	Extra          map[string]string
	Path           string
	Body           string
	Kind           string
	ParseError     string
}

func adrFilePaths(root string) ([]string, error) {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		switch entry.Name() {
		case "README.md", "index.md":
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(files, func(i, j int) bool {
		leftID, leftSlug, leftOK := splitADRFileName(files[i])
		rightID, rightSlug, rightOK := splitADRFileName(files[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return files[i] < files[j]
		}
		if leftID != rightID {
			return leftID < rightID
		}
		return leftSlug < rightSlug
	})
	return files, nil
}

func collectADRs(root string) ([]ADR, error) {
	files, err := adrFilePaths(root)
	if err != nil {
		return nil, err
	}
	var adrs []ADR
	var errs []error
	for _, path := range files {
		adr, err := parseADR(path)
		if err != nil {
			adr = malformedADR(path, err)
			errs = append(errs, fmt.Errorf("%s: %w", relPath(root, path), err))
		}
		adrs = append(adrs, adr)
	}
	sort.Slice(adrs, func(i, j int) bool {
		if adrs[i].ID != adrs[j].ID {
			return adrIDLess(adrs[i].ID, adrs[j].ID)
		}
		return adrs[i].Slug < adrs[j].Slug
	})
	if len(errs) > 0 {
		return adrs, errors.Join(errs...)
	}
	return adrs, nil
}

func parseADR(path string) (ADR, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return ADR{}, err
	}
	return parseADRFromData(data, path)
}

func parseADRFromData(data []byte, path string) (ADR, error) {
	id, slug, ok := splitADRFileName(path)
	if !ok {
		return ADR{}, fmt.Errorf("ADR filename must match NNN-kebab-slug.md")
	}
	paddedID := fmt.Sprintf("%03d", id)
	text := string(data)
	if _, _, hasFrontMatter := splitFrontMatter(text); !hasFrontMatter {
		if strings.HasPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "---\n") {
			return ADR{}, fmt.Errorf("ADR front matter is not closed")
		}
		return parseLegacyADR(text, path, paddedID, slug), nil
	}
	meta, body, err := parseFrontMatter(text)
	if err != nil {
		return ADR{}, err
	}
	if metaID := strings.TrimSpace(meta["id"]); metaID != "" && metaID != paddedID {
		return ADR{}, fmt.Errorf("ADR id %q does not match filename id %s", metaID, paddedID)
	}
	title := headingTitle(body, paddedID)
	body = stripHeading(body, title)
	adr := ADR{
		ID:             paddedID,
		Slug:           slug,
		Title:          title,
		Status:         meta["status"],
		Date:           meta["date"],
		DecisionMakers: meta["decision-makers"],
		Consulted:      meta["consulted"],
		Informed:       meta["informed"],
		Extra:          adrMetaExtra(meta),
		Path:           path,
		Body:           body,
		Kind:           adrKindMADR,
	}
	if adr.Status != "" && !validADRStatus(adr.Status) {
		return ADR{}, fmt.Errorf("unsupported ADR status %q", adr.Status)
	}
	return adr, nil
}

func parseLegacyADR(text string, path string, id string, slug string) ADR {
	title := headingTitle(text, id)
	if strings.HasPrefix(title, "ADR ") {
		prefix := "ADR " + id + ":"
		if strings.HasPrefix(title, prefix) {
			title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	body := stripHeading(text, headingTitle(text, id))
	return ADR{
		ID:     id,
		Slug:   slug,
		Title:  title,
		Path:   path,
		Body:   body,
		Kind:   adrKindLegacy,
		Extra:  map[string]string{},
		Status: legacyBoldValue(text, "Status"),
		Date:   legacyBoldValue(text, "Date"),
	}
}

func malformedADR(path string, err error) ADR {
	id, slug, ok := splitADRFileName(path)
	adr := ADR{
		Path:       path,
		Kind:       adrKindMalformed,
		ParseError: err.Error(),
		Extra:      map[string]string{},
	}
	if ok {
		adr.ID = fmt.Sprintf("%03d", id)
		adr.Slug = slug
	}
	return adr
}

func renderADR(adr ADR) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "status: %s\n", adr.Status)
	fmt.Fprintf(&b, "date: %s\n", adr.Date)
	if adr.DecisionMakers != "" {
		fmt.Fprintf(&b, "decision-makers: %s\n", adr.DecisionMakers)
	}
	if adr.Consulted != "" {
		fmt.Fprintf(&b, "consulted: %s\n", adr.Consulted)
	}
	if adr.Informed != "" {
		fmt.Fprintf(&b, "informed: %s\n", adr.Informed)
	}
	for _, k := range sortedKeys(adr.Extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, adr.Extra[k])
	}
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "# %s\n\n", adr.Title)
	body := strings.TrimSpace(adr.Body)
	if body != "" {
		fmt.Fprintln(&b, body)
		fmt.Fprintln(&b)
	}
	return b.String()
}

func resolveADR(pattern string, adrs []ADR) (ADR, error) {
	normalized, ok := normalizeADRRef(pattern)
	if ok {
		for _, adr := range adrs {
			if adrRef(adr) == normalized || adr.ID == normalized {
				return adr, nil
			}
		}
	}
	for _, adr := range adrs {
		if adr.ID == pattern || adrRef(adr) == pattern {
			return adr, nil
		}
	}
	return ADR{}, fmt.Errorf("ADR %q not found", pattern)
}

func nextADRID(adrs []ADR, root string) string {
	maxID := 0
	for _, adr := range adrs {
		n, err := strconv.Atoi(adr.ID)
		if err == nil && n > maxID {
			maxID = n
		}
	}
	files, err := adrFilePaths(root)
	if err == nil {
		for _, path := range files {
			n, _, ok := splitADRFileName(path)
			if ok && n > maxID {
				maxID = n
			}
		}
	}
	return fmt.Sprintf("%03d", maxID+1)
}

func validADRStatus(status string) bool {
	switch status {
	case "proposed", "accepted", "rejected", "deprecated":
		return true
	default:
		return adrSupersededStatusPattern.MatchString(status)
	}
}

func splitADRFileName(path string) (int, string, bool) {
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	idPart, slug, ok := strings.Cut(base, "-")
	if !ok || len(idPart) != 3 || !validADRSlug(slug) {
		return 0, "", false
	}
	for _, r := range idPart {
		if r < '0' || r > '9' {
			return 0, "", false
		}
	}
	n, err := strconv.Atoi(idPart)
	if err != nil {
		return 0, "", false
	}
	return n, slug, true
}

func validADRSlug(slug string) bool {
	if slug == "" || slug[0] == '-' || slug[len(slug)-1] == '-' {
		return false
	}
	previousDash := false
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z':
			previousDash = false
		case r >= '0' && r <= '9':
			previousDash = false
		case r == '-':
			if previousDash {
				return false
			}
			previousDash = true
		default:
			return false
		}
	}
	return true
}

func normalizeADRRef(ref string) (string, bool) {
	n, suffix, ok := splitADRRef(ref)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%03d%s", n, suffix), true
}

func splitADRRef(ref string) (int, string, bool) {
	i := 0
	for i < len(ref) && ref[i] >= '0' && ref[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	if i > 3 {
		return 0, "", false
	}
	suffix := ref[i:]
	if suffix != "" && !strings.HasPrefix(suffix, "-") {
		return 0, "", false
	}
	n, err := strconv.Atoi(ref[:i])
	if err != nil {
		return 0, "", false
	}
	return n, suffix, true
}

func adrRef(adr ADR) string {
	if adr.Slug == "" {
		return adr.ID
	}
	return adr.ID + "-" + adr.Slug
}

func adrIDLess(a string, b string) bool {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil && an != bn {
		return an < bn
	}
	return a < b
}

func adrMetaExtra(meta map[string]string) map[string]string {
	extra := map[string]string{}
	for k, v := range meta {
		switch k {
		case "id", "status", "date", "decision-makers", "consulted", "informed":
		default:
			extra[k] = v
		}
	}
	return extra
}

func legacyBoldValue(text string, field string) string {
	prefix := "**" + field + ":**"
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

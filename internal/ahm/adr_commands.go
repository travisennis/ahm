package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// adrTransitions defines which source statuses each lifecycle command target
// accepts. The idempotent case (current status == target status) is always
// allowed without consulting this table. Terminal statuses (rejected,
// deprecated, superseded by ADR-NNN) are not listed because they have no valid
// outgoing transition via lifecycle commands.
var adrTransitions = map[string][]string{
	"accepted":   {"proposed"},
	"rejected":   {"proposed"},
	"deprecated": {"accepted"},
	"proposed":   {"accepted"},
}

func (a *app) adrCommand() *cobra.Command {
	adr := &cobra.Command{
		Use:   "adr",
		Short: "Manage Architecture Decision Records",
		Long: `Manage Architecture Decision Records under docs/adr/.

Examples:
  ahm adr create "Choose storage layout"
  ahm adr list
  ahm adr show 009`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("adr requires a subcommand\n  ahm adr <subcommand>")
		},
	}

	createArgs := adrCreateArgs{status: "proposed"}
	create := &cobra.Command{
		Use:   "create <title> [flags]",
		Short: "Create an ADR",
		Long: `Create a new Architecture Decision Record under docs/adr/ and regenerate indexes.

Examples:
  ahm adr create "Choose storage layout"
  ahm adr create "Use Postgres" --status accepted --description "We need a database"
  ahm adr create "Migration plan" --body-file body.md
  ahm --dry-run adr create "Test ADR"`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("adr create requires a title\n  ahm adr create <title>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			createArgs.title = strings.Join(args, " ")
			return a.adrCreateParsed(createArgs)
		},
	}
	create.Flags().StringVar(&createArgs.status, "status", createArgs.status, "Set initial ADR status")
	create.Flags().StringVarP(&createArgs.description, "description", "d", "", "Set ADR context and problem statement")
	create.Flags().StringVar(&createArgs.bodyFile, "body-file", "", "Full Markdown body from a file (or - for stdin); ahm handles ID, front matter, and indexes")
	create.Flags().StringVar(&createArgs.decisionMakers, "decision-makers", "", "Set ADR decision-makers")
	adr.AddCommand(create)

	var listStatuses []string
	list := &cobra.Command{
		Use:   "list",
		Short: "List ADRs",
		Long: `List ADRs parsed from docs/adr/.

Examples:
  ahm adr list
  ahm adr list --status accepted
  ahm --json adr list`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.adrList(listStatuses)
		},
	}
	list.Flags().StringSliceVar(&listStatuses, "status", nil, "Filter ADRs by status (comma-separated or repeatable)")
	adr.AddCommand(list)

	adr.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show an ADR",
		Long: `Show an ADR by ID.

Examples:
  ahm adr show 009
  ahm adr show 9
  ahm --json adr show 009-madr-adr-management`,
		Args: exactArgs(1, "adr show requires an id\n  ahm adr show <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.adrShow(args[0])
		},
	})

	for _, spec := range []struct {
		use    string
		short  string
		long   string
		status string
	}{
		{use: "accept <id>", short: "Accept an ADR", long: `Accept an ADR.

Examples:
  ahm adr accept 009`, status: "accepted"},
		{use: "reject <id>", short: "Reject an ADR", long: `Reject an ADR.

Examples:
  ahm adr reject 009`, status: "rejected"},
		{use: "deprecate <id>", short: "Deprecate an ADR", long: `Deprecate an ADR.

Examples:
  ahm adr deprecate 009`, status: "deprecated"},
		{use: "propose <id>", short: "Propose an ADR", long: `Propose (return to proposed) an ADR.

Examples:
  ahm adr propose 009`, status: "proposed"},
	} {
		status := spec.status
		adr.AddCommand(&cobra.Command{
			Use:   spec.use,
			Short: spec.short,
			Long:  spec.long,
			Args:  exactArgs(1, "adr status command requires an id\n  ahm adr accept|reject|deprecate|propose <id>"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.adrSetStatus(args[0], status)
			},
		})
	}

	var supersedeBy string
	supersede := &cobra.Command{
		Use:   "supersede <old-id> --by <new-id>",
		Short: "Supersede an ADR with another ADR",
		Long: `Supersede an ADR with another ADR.

The old ADR gets status "superseded by ADR-NNN" and a Supersession note.
The replacement ADR gets a cross-reference back.

Examples:
  ahm adr supersede 009 --by 010`,
		Args: exactArgs(1, "adr supersede requires an old id\n  ahm adr supersede <old-id> --by <new-id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.adrSupersede(args[0], supersedeBy)
		},
	}
	supersede.Flags().StringVar(&supersedeBy, "by", "", "Replacement ADR id")
	adr.AddCommand(supersede)

	adr.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Migrate legacy ADRs to MADR front matter",
		Long: `Migrate legacy ADRs to MADR front matter.

CRLF line endings are normalized to LF during migration. This is a side effect of
internal file handling and may appear as line-ending changes in version control
diffs.

Examples:
  ahm adr migrate --dry-run
  ahm adr migrate
  ahm --json adr migrate --dry-run`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.adrMigrate()
		},
	})

	return adr
}

type adrCreateArgs struct {
	title          string
	status         string
	description    string
	bodyFile       string
	decisionMakers string
}

type adrListEntry struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Date   string `json:"date"`
}

func (a *app) adrCreateParsed(parsed adrCreateArgs) error {
	parsed.title = strings.TrimSpace(parsed.title)
	if parsed.title == "" {
		return usageError("adr create requires a title\n  ahm adr create <title>")
	}
	if strings.ContainsAny(parsed.title, "\n\r") {
		return usageError("adr create title must not contain newlines")
	}
	if strings.ContainsAny(parsed.decisionMakers, "\n\r") {
		return usageError("adr create decision-makers must not contain newlines")
	}
	if !validADRCreateStatus(parsed.status) {
		return usageError(fmt.Sprintf("unsupported ADR status %q (supported: %s)", parsed.status, strings.Join(adrCreateStatuses(), ", ")))
	}
	body, err := a.resolveADRCreateBody(parsed)
	if err != nil {
		return err
	}
	body = stripHeading(body, parsed.title)
	if !a.opts.dryRun {
		release, err := acquireWorkflowLock(a.opts.root, "adr-create")
		if err != nil {
			return err
		}
		defer func() {
			if err := release(); err != nil {
				fmt.Fprintln(a.err, err)
			}
		}()
		return a.adrCreateParsedLocked(parsed, body)
	}
	return a.adrCreateParsedLocked(parsed, body)
}

func (a *app) adrCreateParsedLocked(parsed adrCreateArgs, body string) error {
	defer a.emitWarnings()
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		a.addWarning("some ADR files could not be parsed and were skipped")
	}
	id := nextADRID(adrs, a.opts.root)
	slug := adrSlug(parsed.title)
	if slug == "" {
		return usageError("adr create requires a title with letters or digits")
	}
	path := filepath.Join(a.opts.root, "docs", "adr", id+"-"+slug+".md")
	record := ADR{
		ID:             id,
		Slug:           slug,
		Title:          parsed.title,
		Status:         parsed.status,
		Date:           time.Now().Format(time.DateOnly),
		DecisionMakers: strings.TrimSpace(parsed.decisionMakers),
		Extra:          map[string]string{},
		Path:           path,
		Body:           body,
		Kind:           adrKindMADR,
	}
	if a.opts.dryRun {
		return a.emit(map[string]any{"create": path, "id": id})
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("adr id %s already exists at %s; retry adr create", id, relPath(a.opts.root, path))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking adr path %s: %w", relPath(a.opts.root, path), err)
	}
	if err := writeFileAtomic(path, []byte(renderADR(record)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, id)
	return nil
}

func (a *app) adrList(statuses []string) error {
	defer a.emitWarnings()
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		a.addWarning("some ADR files could not be parsed and were skipped")
	}
	filtered := filterReadableADRs(adrs)
	if len(statuses) > 0 {
		filtered = filterADRsByStatus(filtered, statuses)
	}
	entries := adrListEntries(filtered)
	if a.opts.json || a.opts.plain {
		return a.emit(entries)
	}
	for _, entry := range entries {
		fmt.Fprintf(a.out, "%s [%s] %s %s\n", entry.ID, entry.Status, entry.Date, entry.Title)
	}
	return nil
}

func (a *app) adrShow(id string) error {
	defer a.emitWarnings()
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		a.addWarning("some ADR files could not be parsed and were skipped")
	}
	adr, err := resolveADR(id, filterReadableADRs(adrs))
	if err != nil {
		return err
	}
	if a.opts.json || a.opts.plain {
		return a.emit(adr)
	}
	data, err := os.ReadFile(adr.Path)
	if err != nil {
		return err
	}
	_, err = a.out.Write(data)
	return err
}

func (a *app) adrSetStatus(id string, status string) error {
	defer a.emitWarnings()
	adr, err := a.resolveMutableADR(id)
	if err != nil {
		return err
	}
	if adr.Status != status {
		allowed, ok := adrTransitions[status]
		if !ok || !containsString(allowed, adr.Status) {
			return fmt.Errorf("ADR %s is %s; cannot set to %s", adr.ID, adr.Status, status)
		}
	}
	today := time.Now().Format(time.DateOnly)
	if a.opts.dryRun {
		return a.emit(map[string]any{"adr": adr.ID, "status": status, "date": today})
	}
	if err := rewriteADRFrontMatter(adr.Path, map[string]string{
		"status": status,
		"date":   today,
	}); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", adr.ID, status)
	return nil
}

func (a *app) adrSupersede(oldID string, newID string) error {
	defer a.emitWarnings()
	newID = strings.TrimSpace(newID)
	if newID == "" {
		return usageError("adr supersede requires --by\n  ahm adr supersede <old-id> --by <new-id>")
	}
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		a.addWarning("some ADR files could not be parsed and were skipped")
	}
	readable := filterReadableADRs(adrs)
	oldADR, err := resolveADR(oldID, readable)
	if err != nil {
		return err
	}
	newADR, err := resolveADR(newID, readable)
	if err != nil {
		return err
	}
	if oldADR.ID == newADR.ID {
		return usageError("adr supersede cannot supersede an ADR with itself")
	}
	if oldADR.Kind != adrKindMADR {
		return fmt.Errorf("ADR %s is not a MADR record", oldADR.ID)
	}
	if newADR.Kind != adrKindMADR {
		return fmt.Errorf("ADR %s is not a MADR record", newADR.ID)
	}
	nextStatus := "superseded by ADR-" + newADR.ID
	if strings.HasPrefix(strings.ToLower(oldADR.Status), "superseded by adr-") && oldADR.Status != nextStatus {
		return fmt.Errorf("ADR %s is already %s", oldADR.ID, oldADR.Status)
	}
	if oldADR.Status != "accepted" && !strings.HasPrefix(strings.ToLower(oldADR.Status), "superseded by adr-") {
		return fmt.Errorf("ADR %s is %s; only accepted ADRs can be superseded", oldADR.ID, oldADR.Status)
	}
	if newADR.Status != "accepted" {
		return fmt.Errorf("replacement ADR %s is %s; only accepted ADRs can be the replacement", newADR.ID, newADR.Status)
	}
	today := time.Now().Format(time.DateOnly)
	if a.opts.dryRun {
		return a.emit(map[string]any{"adr": oldADR.ID, "status": nextStatus, "by": newADR.ID, "date": today})
	}
	if err := rewriteADR(oldADR.Path, map[string]string{
		"status": nextStatus,
		"date":   today,
	}, func(body string) string {
		return upsertADRSupersessionNote(body, newADR)
	}); err != nil {
		return err
	}
	if err := rewriteADR(newADR.Path, nil, func(body string) string {
		return upsertADRMoreInformationReference(body, oldADR)
	}); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", oldADR.ID, nextStatus)
	return nil
}

func (a *app) resolveMutableADR(id string) (ADR, error) {
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		a.addWarning("some ADR files could not be parsed and were skipped")
	}
	adr, err := resolveADR(id, filterReadableADRs(adrs))
	if err != nil {
		return ADR{}, err
	}
	if adr.Kind != adrKindMADR {
		return ADR{}, fmt.Errorf("ADR %s is not a MADR record", adr.ID)
	}
	return adr, nil
}

func (a *app) resolveADRCreateBody(parsed adrCreateArgs) (string, error) {
	if parsed.bodyFile == "" {
		description := strings.TrimSpace(parsed.description)
		if description == "" {
			description = "TODO."
		}
		return defaultADRBody(description), nil
	}
	if parsed.description != "" {
		return "", usageError("adr create supports --body-file or --description, not both")
	}
	var (
		data   []byte
		err    error
		source string
	)
	if parsed.bodyFile == "-" {
		source = "stdin"
		if a.in == nil {
			return "", usageError("adr create --body-file - requires stdin")
		}
		data, err = io.ReadAll(a.in)
	} else {
		source = parsed.bodyFile
		data, err = os.ReadFile(parsed.bodyFile)
	}
	if err != nil {
		return "", fmt.Errorf("reading ADR body from %s: %w", source, err)
	}
	body := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	if body == "" {
		return "", usageError(fmt.Sprintf("ADR body from %s is empty", source))
	}
	return body, nil
}

func defaultADRBody(description string) string {
	return "## Context and Problem Statement\n\n" + description + "\n\n" +
		"## Decision Drivers\n\n- TODO\n\n" +
		"## Considered Options\n\n- TODO\n\n" +
		"## Decision Outcome\n\nChosen option: TODO, because TODO.\n\n" +
		"### Consequences\n\n- Good, because TODO.\n- Bad, because TODO.\n\n" +
		"## More Information\n\n- TODO\n"
}

func adrCreateStatuses() []string {
	return []string{"proposed", "accepted", "rejected", "deprecated"}
}

func validADRCreateStatus(status string) bool {
	return containsString(adrCreateStatuses(), status)
}

func adrSlug(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	previousDash := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			previousDash = false
		default:
			if b.Len() > 0 && !previousDash {
				b.WriteByte('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func filterReadableADRs(adrs []ADR) []ADR {
	out := make([]ADR, 0, len(adrs))
	for _, adr := range adrs {
		if adr.Kind == adrKindMalformed {
			continue
		}
		out = append(out, adr)
	}
	return out
}

func filterADRsByStatus(adrs []ADR, statuses []string) []ADR {
	allowed := make(map[string]bool, len(statuses))
	for _, raw := range statuses {
		status := strings.ToLower(strings.TrimSpace(raw))
		if status != "" {
			allowed[status] = true
		}
	}
	if len(allowed) == 0 {
		return adrs
	}
	out := make([]ADR, 0, len(adrs))
	for _, adr := range adrs {
		status := strings.ToLower(strings.TrimSpace(adr.Status))
		for allowedStatus := range allowed {
			if status == allowedStatus || strings.HasPrefix(status, allowedStatus+" ") {
				out = append(out, adr)
				break
			}
		}
	}
	return out
}

func adrListEntries(adrs []ADR) []adrListEntry {
	entries := make([]adrListEntry, 0, len(adrs))
	for _, adr := range adrs {
		entries = append(entries, adrListEntry{
			ID:     adr.ID,
			Title:  adr.Title,
			Status: adr.Status,
			Date:   adr.Date,
		})
	}
	return entries
}

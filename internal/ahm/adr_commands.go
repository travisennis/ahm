package ahm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) adrCommand() *cobra.Command {
	adr := &cobra.Command{
		Use:   "adr",
		Short: "Manage Architecture Decision Records",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("adr requires a subcommand")
		},
	}

	createArgs := adrCreateArgs{status: "proposed"}
	create := &cobra.Command{
		Use:   "create <title> [flags]",
		Short: "Create an ADR",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("adr create requires a title")
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

	return adr
}

type adrCreateArgs struct {
	title          string
	status         string
	description    string
	bodyFile       string
	decisionMakers string
}

func (a *app) adrCreateParsed(parsed adrCreateArgs) error {
	parsed.title = strings.TrimSpace(parsed.title)
	if parsed.title == "" {
		return usageError("adr create requires a title")
	}
	if !validADRCreateStatus(parsed.status) {
		return usageError(fmt.Sprintf("unsupported ADR status %q (supported: %s)", parsed.status, strings.Join(adrCreateStatuses(), ", ")))
	}
	body, err := a.resolveADRCreateBody(parsed)
	if err != nil {
		return err
	}
	body = stripHeading(body, parsed.title)
	adrs, err := collectADRs(a.opts.root)
	if err != nil {
		fmt.Fprintln(a.err, "warning: some ADR files could not be parsed and were skipped")
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
	if err := writeFileAtomic(path, []byte(renderADR(record)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, id)
	return nil
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

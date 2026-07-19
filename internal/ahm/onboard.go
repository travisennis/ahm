package ahm

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const onboardSnippet = `## Managed Work (ahm)

This project manages tasks, ExecPlans, and research notes with ` + "`ahm`" + `;
workflow records live under ` + "`.ahm/`" + `. ADRs are project-owned durable
documentation under ` + "`docs/adr/`" + `, with lifecycle managed through ` + "`ahm`" + `.

ALWAYS run ` + "`ahm prime`" + ` before starting work, and re-run it after context
compaction. It reports workflow state, in-progress and ready tasks,
validation warnings, and which scoped ` + "`ahm context <scope>`" + ` command covers
the work at hand. Work done without it often conflicts with tracked
in-progress work.

Never hand-edit ahm-generated indexes; update source records and run the
appropriate ` + "`ahm`" + ` command. Use ` + "`ahm task`" + ` for task state changes and
` + "`ahm adr`" + ` for ADR lifecycle changes.`

type onboardReport struct {
	Snippet string `json:"snippet"`
}

func (r onboardReport) RenderText(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "Paste this snippet into the project-owned AGENTS.md. Repositories using CLAUDE.md typically import AGENTS.md."); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%s\n", r.Snippet)
	return err
}

func (a *app) onboardCommand() *cobra.Command {
	return &cobra.Command{Use: "onboard", Short: "Print the AGENTS.md bootstrap snippet", Long: `Print a minimal paste-ready AGENTS.md snippet.

The command never writes AGENTS.md. Text mode includes brief framing,
--plain prints only the snippet, and --json returns it as a field.`, Args: noArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.detectRootOrCWD(); err != nil {
			return err
		}
		if a.opts.plain {
			_, err := fmt.Fprintf(a.out, "%s\n", onboardSnippet)
			return err
		}
		return a.emit(onboardReport{Snippet: onboardSnippet})
	}}
}

package ahm

import (
	"fmt"
	"io"
)

type validationReport struct {
	OK       bool                `json:"ok"`
	Errors   []validationFinding `json:"errors"`
	Warnings []validationFinding `json:"warnings"`
	Info     []validationFinding `json:"info"`

	// cache holds records already read within this command so each ADR and
	// generated index is read from disk at most once. It is scoped to the report
	// so concurrent validation runs and tests do not share state. Mutation paths
	// hand in the cache their index generation filled; every other path gets a
	// fresh one and reads everything from disk.
	cache *recordCache

	// meta carries the command's shared configuration cache, so every validator
	// in this report observes one .ahm/config.json even if the file changes
	// mid-run. A nil cache reads the configuration from disk on each call.
	meta *metadataCache
}

type validationFinding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// RenderText implements the textRenderer interface for validationReport.
func (r validationReport) RenderText(w io.Writer) error {
	if r.OK && len(r.Errors) == 0 && len(r.Warnings) == 0 && len(r.Info) == 0 {
		_, err := fmt.Fprintln(w, "ok")
		return err
	}
	if _, err := fmt.Fprintf(w, "ok: %v\n", r.OK); err != nil {
		return err
	}
	if len(r.Errors) > 0 {
		if _, err := fmt.Fprintln(w, "errors:"); err != nil {
			return err
		}
		for _, e := range r.Errors {
			if e.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", e.Code, e.Message, e.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", e.Code, e.Message); err != nil {
					return err
				}
			}
		}
	}
	if len(r.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "warnings:"); err != nil {
			return err
		}
		for _, wrn := range r.Warnings {
			if wrn.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", wrn.Code, wrn.Message, wrn.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", wrn.Code, wrn.Message); err != nil {
					return err
				}
			}
		}
	}
	if len(r.Info) > 0 {
		if _, err := fmt.Fprintln(w, "info:"); err != nil {
			return err
		}
		for _, i := range r.Info {
			if i.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", i.Code, i.Message, i.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", i.Code, i.Message); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func newValidationReport(cache *recordCache, meta *metadataCache) validationReport {
	return validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}, cache: cache, meta: meta}
}

// readMetadata reads the committed configuration for root through the run's
// shared cache, so every validator in this report observes the same bytes. A
// nil cache reads through to disk.
func (r *validationReport) readMetadata(root string) (metadata, error) {
	return r.meta.read(root)
}

func (r *validationReport) addError(code string, path string, message string) {
	r.Errors = append(r.Errors, validationFinding{Code: code, Path: path, Message: message})
}

func (r *validationReport) addWarning(code string, path string, message string) {
	r.Warnings = append(r.Warnings, validationFinding{Code: code, Path: path, Message: message})
}

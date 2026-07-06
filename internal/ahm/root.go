package ahm

import (
	"fmt"
	"os"
	"path/filepath"
)

func (a *app) detectRoot() error {
	if a.opts.root != "" {
		return nil
	}
	root, err := detectManagedRoot()
	if err != nil {
		return err
	}
	a.opts.root = root
	return nil
}

func (a *app) detectRootOrCWD() error {
	if a.opts.root != "" {
		return nil
	}
	root, err := detectManagedRoot()
	if err != nil {
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	a.opts.root = root
	return nil
}

func detectManagedRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if stat, err := os.Stat(filepath.Join(dir, ".ahm", "config.json")); err == nil && !stat.IsDir() {
			return dir, nil
		}
		if stat, err := os.Stat(filepath.Join(dir, ".agents", "ahm.json")); err == nil && !stat.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a managed repository (no .git, .ahm/config.json, or .agents/ahm.json found); use --root to specify a directory or run 'ahm init' to create a workflow")
		}
		dir = parent
	}
}

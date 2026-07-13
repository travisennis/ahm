package ahm

import (
	"os"
	"strings"
)

var gitRepositoryEnvironment = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_COMMON_DIR",
}

func cleanGitEnvironment() []string {
	env := os.Environ()
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if isGitRepositoryEnvironment(name) {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}

func isGitRepositoryEnvironment(name string) bool {
	for _, blocked := range gitRepositoryEnvironment {
		if strings.EqualFold(name, blocked) {
			return true
		}
	}
	return false
}

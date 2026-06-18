package ahm

import (
	"fmt"
	"strings"
)

func validateTaskCreateEnums(parsed taskCreateArgs) error {
	if !validTaskStatus(parsed.status) {
		return usageError(enumError("status", parsed.status, statusOrder()))
	}
	if !validTaskPriority(parsed.priority) {
		return usageError(enumError("priority", parsed.priority, priorityOrder()))
	}
	if !validTaskEffort(parsed.effort) {
		return usageError(enumError("effort", parsed.effort, effortOrder()))
	}
	return nil
}

func validateTaskEnums(task Task, source string) error {
	if !validTaskStatus(task.Status) {
		return fmt.Errorf("%s: %s", source, enumError("status", task.Status, statusOrder()))
	}
	if !validTaskPriority(task.Priority) {
		return fmt.Errorf("%s: %s", source, enumError("priority", task.Priority, priorityOrder()))
	}
	if !validTaskEffort(task.Effort) {
		return fmt.Errorf("%s: %s", source, enumError("effort", task.Effort, effortOrder()))
	}
	return nil
}

func enumError(field string, value string, allowed []string) string {
	return fmt.Sprintf("unsupported task %s %q (supported: %s)", field, value, strings.Join(allowed, ", "))
}

func normalizeTaskStatus(status string) (string, error) {
	key := enumKey(status)
	for _, item := range statusOrder() {
		if enumKey(item) == key {
			return item, nil
		}
	}
	return "", usageError(enumError("status", status, statusOrder()))
}

func enumKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(value)
}

func validTaskStatus(status string) bool {
	return containsString(statusOrder(), status)
}

func validTaskPriority(priority string) bool {
	return containsString(priorityOrder(), priority)
}

func validTaskEffort(effort string) bool {
	return containsString(effortOrder(), effort)
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

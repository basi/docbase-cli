package format

import (
	"strings"

	"github.com/basi/docbase-cli/pkg/docbase"
)

// Tags formats tags for display
func Tags(tags []docbase.Tag) string {
	var tagNames []string
	for _, tag := range tags {
		tagNames = append(tagNames, tag.Name)
	}
	return strings.Join(tagNames, ", ")
}

// Groups formats groups for display
func Groups(groups []docbase.Group) string {
	var groupNames []string
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}
	return strings.Join(groupNames, ", ")
}

// Truncate truncates a string to the specified length (rune-aware for multibyte characters)
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if maxLen <= 3 {
		return s[:min(len(s), maxLen)]
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

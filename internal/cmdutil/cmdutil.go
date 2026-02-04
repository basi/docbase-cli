package cmdutil

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/pkg/docbase"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// CheckRequiredFlags checks if the required flags are set
func CheckRequiredFlags(cmd *cobra.Command, flagNames ...string) error {
	for _, flagName := range flagNames {
		flag := cmd.Flag(flagName)
		if flag == nil {
			return fmt.Errorf("flag %s not found", flagName)
		}
		if flag.Value.String() == "" {
			return fmt.Errorf("required flag %s not set", flagName)
		}
	}
	return nil
}

// CreateClient creates a DocBase API client
func CreateClient(cmd *cobra.Command) (*docbase.API, error) {
	teamDomain := config.GetTeamDomain(cmd.Flag("team").Value.String())
	accessToken := config.GetAccessToken(cmd.Flag("token").Value.String())

	if teamDomain == "" {
		return nil, fmt.Errorf("team domain is required")
	}

	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}

	return docbase.NewAPI(teamDomain, accessToken), nil
}

// ReadFile reads a file and returns its content
func ReadFile(filePath string) (string, error) {
	if filePath == "-" {
		// Read from stdin
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(bytes), nil
	}

	// Read from file
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return string(bytes), nil
}

// PrintError prints an error message
func PrintError(err error) {
	fmt.Fprintln(os.Stderr, color.RedString("Error: %s", err.Error()))
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	fmt.Println(color.GreenString("Success: %s", message))
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	fmt.Println(color.YellowString("Warning: %s", message))
}

// PrintInfo prints an info message
func PrintInfo(message string) {
	fmt.Println(color.BlueString("Info: %s", message))
}

// FormatTags formats tags for display
func FormatTags(tags []docbase.Tag) string {
	var tagNames []string
	for _, tag := range tags {
		tagNames = append(tagNames, tag.Name)
	}
	return strings.Join(tagNames, ", ")
}

// FormatGroups formats groups for display
func FormatGroups(groups []docbase.Group) string {
	var groupNames []string
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}
	return strings.Join(groupNames, ", ")
}

// BuildGroupNameToIDMap retrieves all groups and returns a map of group name -> group ID.
func BuildGroupNameToIDMap(client *docbase.API) (map[string]int, error) {
	groupMap := make(map[string]int)
	page := 1
	perPage := 200

	for {
		groups, err := client.Group.List(page, perPage)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve group list: %w", err)
		}

		for _, group := range groups.Groups {
			groupMap[group.Name] = group.ID
		}

		if len(groups.Groups) < perPage {
			break
		}
		page++
	}

	return groupMap, nil
}

// ResolveGroupIDsFromMap resolves group names to IDs using a pre-built map.
func ResolveGroupIDsFromMap(groupMap map[string]int, groupNames []string) ([]int, error) {
	if len(groupNames) == 0 {
		return nil, nil
	}

	groupIDs := make([]int, 0, len(groupNames))
	for _, name := range groupNames {
		id, ok := groupMap[name]
		if !ok {
			availableGroups := make([]string, 0, len(groupMap))
			for groupName := range groupMap {
				availableGroups = append(availableGroups, groupName)
			}
			sort.Strings(availableGroups)
			return nil, fmt.Errorf("group not found: %s\nAvailable groups: %s", name, strings.Join(availableGroups, ", "))
		}
		groupIDs = append(groupIDs, id)
	}

	return groupIDs, nil
}

// ResolveGroupIDs resolves group names to group IDs
func ResolveGroupIDs(client *docbase.API, groupNames []string) ([]int, error) {
	if len(groupNames) == 0 {
		return nil, nil
	}

	groupMap, err := BuildGroupNameToIDMap(client)
	if err != nil {
		return nil, err
	}

	return ResolveGroupIDsFromMap(groupMap, groupNames)
}

// TruncateString truncates a string to the specified length (rune-aware for multibyte characters)
func TruncateString(s string, maxLen int) string {
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

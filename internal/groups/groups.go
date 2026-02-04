package groups

import (
	"fmt"
	"sort"
	"strings"

	"github.com/basi/docbase-cli/pkg/docbase"
)

// BuildNameToIDMap retrieves all groups and returns a map of group name -> group ID.
func BuildNameToIDMap(client *docbase.API) (map[string]int, error) {
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

// ResolveIDsFromMap resolves group names to IDs using a pre-built map.
func ResolveIDsFromMap(groupMap map[string]int, groupNames []string) ([]int, error) {
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

// ResolveIDs resolves group names to group IDs
func ResolveIDs(client *docbase.API, groupNames []string) ([]int, error) {
	if len(groupNames) == 0 {
		return nil, nil
	}

	groupMap, err := BuildNameToIDMap(client)
	if err != nil {
		return nil, err
	}

	return ResolveIDsFromMap(groupMap, groupNames)
}

package group

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/formatter"
	"github.com/basi/docbase-cli/internal/utils"
	"github.com/spf13/cobra"
)

var (
	// GroupCmd represents the group command
	GroupCmd = &cobra.Command{
		Use:   "group",
		Short: "Manage groups",
		Long:  `Manage groups in DocBase.`,
	}

	// ListCmd represents the group list command
	ListCmd = &cobra.Command{
		Use:   "list",
		Short: "List groups",
		Long: `List groups in DocBase.

Example:
  docbase group list
  docbase group list --page 2 --per-page 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")

			groupList, err := client.Group.List(page, perPage)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for list
				fmt.Printf("Count: %d\n", len(groupList.Groups))
				fmt.Println(strings.Repeat("-", 80))
				fmt.Printf("%-8s %s\n", "ID", "Name")
				fmt.Println(strings.Repeat("-", 80))

				for _, group := range groupList.Groups {
					fmt.Printf("%-8d %s\n",
						group.ID,
						utils.TruncateString(group.Name, 37),
					)
				}

				if len(groupList.Groups) == perPage {
					fmt.Printf("\nUse --page %d to see the next page\n", page+1)
				}
				return nil
			}

			return f.Print(groupList)
		},
	}

	// ViewCmd represents the group view command
	ViewCmd = &cobra.Command{
		Use:   "view [id]",
		Short: "View a group",
		Long: `View a group in DocBase.

Example:
  docbase group view 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid group ID: %s", args[0])
			}

			group, err := client.Group.Get(id)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for view
				fmt.Printf("ID: %d\n", group.ID)
				fmt.Printf("Name: %s\n", group.Name)
				fmt.Printf("Created At: %s\n", group.CreatedAt.Format("2006-01-02 15:04:05"))
				if group.Description != "" {
					fmt.Printf("Description: %s\n", group.Description)
				}
				fmt.Printf("Posts Count: %d\n", group.PostsCount)
				if group.LastActivityAt != nil {
					fmt.Printf("Last Activity At: %s\n", group.LastActivityAt.Format("2006-01-02 15:04:05"))
				}
				fmt.Printf("Members: %d\n", len(group.Users))
				return nil
			}

			return f.Print(group)
		},
	}

	// MembersCmd represents the group members command
	MembersCmd = &cobra.Command{
		Use:   "members [id]",
		Short: "List members of a group",
		Long: `List members of a group in DocBase.

Example:
  docbase group members 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid group ID: %s", args[0])
			}

			members, err := client.Group.GetMembers(id)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for members
				fmt.Printf("Group ID: %d\n", id)
				fmt.Printf("Total Members: %d\n", len(members))
				fmt.Println(strings.Repeat("-", 80))
				fmt.Printf("%-8s %-30s %-8s %s\n", "ID", "Name", "Admin", "Profile Image URL")
				fmt.Println(strings.Repeat("-", 80))

				for _, member := range members {
					fmt.Printf("%-8d %-30s %-8t %s\n",
						member.ID,
						utils.TruncateString(member.Name, 27),
						member.Admin,
						utils.TruncateString(member.ProfileImageURL, 40),
					)
				}
				return nil
			}

			return f.Print(members)
		},
	}
)

func init() {
	// Add group command to root command
	root.AddCommand(GroupCmd)

	// Add subcommands to group command
	GroupCmd.AddCommand(ListCmd)
	GroupCmd.AddCommand(ViewCmd)
	GroupCmd.AddCommand(MembersCmd)

	// Add flags to list command
	ListCmd.Flags().Int("page", 1, "Page number")
	ListCmd.Flags().Int("per-page", 20, "Number of items per page")
}

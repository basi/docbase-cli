package tag

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/internal/formatter"
	"github.com/spf13/cobra"
)

var (
	// TagCmd represents the tag command
	TagCmd = &cobra.Command{
		Use:   "tag",
		Short: "Manage tags",
		Long:  `Manage tags in DocBase.`,
	}

	// ListCmd represents the tag list command
	ListCmd = &cobra.Command{
		Use:   "list",
		Short: "List tags",
		Long: `List tags in DocBase.

Example:
  docbase tag list
  docbase tag list --page 2 --per-page 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")

			tagList, err := c.Tag.List(page, perPage)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for list
				fmt.Printf("Total: %d\n", tagList.Meta.Total)
				fmt.Println(strings.Repeat("-", 80))
				fmt.Println("Tag Name")
				fmt.Println(strings.Repeat("-", 80))

				for _, t := range tagList.Tags {
					fmt.Println(t.Name)
				}

				if tagList.Meta.NextPage != nil {
					nextPage, _ := strconv.Atoi(*tagList.Meta.NextPage)
					fmt.Printf("\nUse --page %d to see the next page\n", nextPage)
				}
				return nil
			}

			return f.Print(tagList)
		},
	}

	// SearchCmd represents the tag search command
	SearchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search tags",
		Long: `Search tags in DocBase.

Example:
  docbase tag search "weekly"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			query := args[0]
			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")

			tagList, err := c.Tag.Search(query, page, perPage)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for search
				fmt.Printf("Search Query: %s\n", query)
				fmt.Printf("Total: %d\n", tagList.Meta.Total)
				fmt.Println(strings.Repeat("-", 80))
				fmt.Println("Tag Name")
				fmt.Println(strings.Repeat("-", 80))

				for _, t := range tagList.Tags {
					fmt.Println(t.Name)
				}

				if tagList.Meta.NextPage != nil {
					nextPage, _ := strconv.Atoi(*tagList.Meta.NextPage)
					fmt.Printf("\nUse --page %d to see the next page\n", nextPage)
				}
				return nil
			}

			return f.Print(tagList)
		},
	}
)

func init() {
	// Add tag command to root command
	root.AddCommand(TagCmd)

	// Add subcommands to tag command
	TagCmd.AddCommand(ListCmd)
	TagCmd.AddCommand(SearchCmd)

	// Add flags to list command
	ListCmd.Flags().Int("page", 1, "Page number")
	ListCmd.Flags().Int("per-page", 20, "Number of items per page")

	// Add flags to search command
	SearchCmd.Flags().Int("page", 1, "Page number")
	SearchCmd.Flags().Int("per-page", 20, "Number of items per page")
}

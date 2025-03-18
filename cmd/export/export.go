package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/utils"
	"github.com/basi/docbase-cli/pkg/docbase"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// ExportCmd represents the export command
	ExportCmd = &cobra.Command{
		Use:   "export",
		Short: "Export memos",
		Long:  `Export memos from DocBase to local files.`,
	}

	// GroupCmd represents the export group command
	GroupCmd = &cobra.Command{
		Use:   "group [group_name]",
		Short: "Export memos from a group",
		Long: `Export memos from a group in DocBase to local files.

Example:
  docbase export group "全員"
  docbase export group "開発" --output ./exports --format md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			groupName := args[0]
			outputDir, _ := cmd.Flags().GetString("output")
			format, _ := cmd.Flags().GetString("format")
			limit, _ := cmd.Flags().GetInt("limit")

			// Create output directory if it doesn't exist
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Build query
			query := fmt.Sprintf("group:%s", groupName)

			// Get memos
			page := 1
			perPage := 100
			count := 0

			for {
				fmt.Printf("Fetching page %d...\n", page)
				memoList, err := client.Memo.List(page, perPage, query)
				if err != nil {
					return err
				}

				if len(memoList.Memos) == 0 {
					break
				}

				for _, memo := range memoList.Memos {
					if limit > 0 && count >= limit {
						fmt.Println("Reached limit, stopping export")
						return nil
					}

					// Get full memo details
					fullMemo, err := client.Memo.Get(memo.ID)
					if err != nil {
						fmt.Printf("Error getting memo %d: %v, skipping\n", memo.ID, err)
						continue
					}

					// Create filename
					var filename string
					switch format {
					case "md":
						filename = fmt.Sprintf("%d_%s.md", fullMemo.ID, sanitizeFilename(fullMemo.Title))
					case "json":
						filename = fmt.Sprintf("%d_%s.json", fullMemo.ID, sanitizeFilename(fullMemo.Title))
					default:
						return fmt.Errorf("unsupported format: %s", format)
					}

					filepath := filepath.Join(outputDir, filename)

					// Write file
					if err := writeMemoToFile(fullMemo, filepath, format); err != nil {
						fmt.Printf("Error writing memo %d to file: %v, skipping\n", fullMemo.ID, err)
						continue
					}

					fmt.Printf("Exported memo %d to %s\n", fullMemo.ID, filepath)
					count++

					// Sleep to avoid rate limiting
					time.Sleep(1 * time.Second)
				}

				if memoList.Meta.NextPage == nil {
					break
				}
				page = *memoList.Meta.NextPage
			}

			fmt.Println(color.GreenString("Export completed successfully"))
			fmt.Printf("Exported %d memos to %s\n", count, outputDir)
			return nil
		},
	}

	// TagCmd represents the export tag command
	TagCmd = &cobra.Command{
		Use:   "tag [tag_name]",
		Short: "Export memos with a tag",
		Long: `Export memos with a tag in DocBase to local files.

Example:
  docbase export tag "週報"
  docbase export tag "開発" --output ./exports --format md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			tagName := args[0]
			outputDir, _ := cmd.Flags().GetString("output")
			format, _ := cmd.Flags().GetString("format")
			limit, _ := cmd.Flags().GetInt("limit")

			// Create output directory if it doesn't exist
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Build query
			query := fmt.Sprintf("tag:%s", tagName)

			// Get memos
			page := 1
			perPage := 100
			count := 0

			for {
				fmt.Printf("Fetching page %d...\n", page)
				memoList, err := client.Memo.List(page, perPage, query)
				if err != nil {
					return err
				}

				if len(memoList.Memos) == 0 {
					break
				}

				for _, memo := range memoList.Memos {
					if limit > 0 && count >= limit {
						fmt.Println("Reached limit, stopping export")
						return nil
					}

					// Get full memo details
					fullMemo, err := client.Memo.Get(memo.ID)
					if err != nil {
						fmt.Printf("Error getting memo %d: %v, skipping\n", memo.ID, err)
						continue
					}

					// Create filename
					var filename string
					switch format {
					case "md":
						filename = fmt.Sprintf("%d_%s.md", fullMemo.ID, sanitizeFilename(fullMemo.Title))
					case "json":
						filename = fmt.Sprintf("%d_%s.json", fullMemo.ID, sanitizeFilename(fullMemo.Title))
					default:
						return fmt.Errorf("unsupported format: %s", format)
					}

					filepath := filepath.Join(outputDir, filename)

					// Write file
					if err := writeMemoToFile(fullMemo, filepath, format); err != nil {
						fmt.Printf("Error writing memo %d to file: %v, skipping\n", fullMemo.ID, err)
						continue
					}

					fmt.Printf("Exported memo %d to %s\n", fullMemo.ID, filepath)
					count++

					// Sleep to avoid rate limiting
					time.Sleep(1 * time.Second)
				}

				if memoList.Meta.NextPage == nil {
					break
				}
				page = *memoList.Meta.NextPage
			}

			fmt.Println(color.GreenString("Export completed successfully"))
			fmt.Printf("Exported %d memos to %s\n", count, outputDir)
			return nil
		},
	}
)

// sanitizeFilename sanitizes a string to be used as a filename
func sanitizeFilename(s string) string {
	// Replace invalid characters with underscore
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Limit length
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

// writeMemoToFile writes a memo to a file in the specified format
func writeMemoToFile(memo *docbase.Memo, filepath string, format string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	switch format {
	case "md":
		// Write frontmatter
		fmt.Fprintf(file, "---\n")
		fmt.Fprintf(file, "id: %d\n", memo.ID)
		fmt.Fprintf(file, "title: \"%s\"\n", memo.Title)
		fmt.Fprintf(file, "author: \"%s\"\n", memo.User.Name)
		fmt.Fprintf(file, "created_at: %s\n", memo.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(file, "updated_at: %s\n", memo.UpdatedAt.Format(time.RFC3339))
		fmt.Fprintf(file, "url: %s\n", memo.URL)
		fmt.Fprintf(file, "draft: %t\n", memo.Draft)
		fmt.Fprintf(file, "archived: %t\n", memo.Archived)
		fmt.Fprintf(file, "scope: %s\n", memo.Scope)

		// Tags
		fmt.Fprintf(file, "tags:\n")
		for _, tag := range memo.Tags {
			fmt.Fprintf(file, "  - %s\n", tag.Name)
		}

		// Groups
		fmt.Fprintf(file, "groups:\n")
		for _, group := range memo.Groups {
			fmt.Fprintf(file, "  - %s\n", group.Name)
		}

		fmt.Fprintf(file, "---\n\n")

		// Write body
		fmt.Fprintf(file, "%s\n", memo.Body)

	case "json":
		// Use json.Marshal to convert memo to JSON
		bytes, err := json.MarshalIndent(memo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(file, "%s\n", string(bytes))

	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	return nil
}

func init() {
	// Add export command to root command
	root.AddCommand(ExportCmd)

	// Add subcommands to export command
	ExportCmd.AddCommand(GroupCmd)
	ExportCmd.AddCommand(TagCmd)

	// Add flags to group command
	GroupCmd.Flags().String("output", "./exports", "Output directory")
	GroupCmd.Flags().String("format", "md", "Output format (md, json)")
	GroupCmd.Flags().Int("limit", 0, "Limit the number of memos to export (0 for no limit)")

	// Add flags to tag command
	TagCmd.Flags().String("output", "./exports", "Output directory")
	TagCmd.Flags().String("format", "md", "Output format (md, json)")
	TagCmd.Flags().Int("limit", 0, "Limit the number of memos to export (0 for no limit)")
}
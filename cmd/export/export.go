package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/pkg/docbase"
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
			query := fmt.Sprintf("group:%s", args[0])
			return runExport(cmd, query)
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
			query := fmt.Sprintf("tag:%s", args[0])
			return runExport(cmd, query)
		},
	}
)

// runExport is the common export logic for both group and tag commands
func runExport(cmd *cobra.Command, query string) error {
	c, err := client.Create(cmd)
	if err != nil {
		return err
	}

	outputDir, _ := cmd.Flags().GetString("output")
	format, _ := cmd.Flags().GetString("format")
	limit, _ := cmd.Flags().GetInt("limit")

	// Validate format early
	if format != "md" && format != "json" {
		return fmt.Errorf("unsupported format: %s", format)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get memos
	page := 1
	perPage := 100
	count := 0

	for {
		fmt.Printf("Fetching page %d...\n", page)
		memoList, err := c.Memo.List(page, perPage, query)
		if err != nil {
			return err
		}

		if len(memoList.Memos) == 0 {
			break
		}

		for _, memo := range memoList.Memos {
			if limit > 0 && count >= limit {
				fmt.Println("Reached limit, stopping export")
				fmt.Println(color.GreenString("Export completed successfully"))
				fmt.Printf("Exported %d memos to %s\n", count, outputDir)
				return nil
			}

			// Get full memo details
			fullMemo, err := c.Memo.Get(memo.ID)
			if err != nil {
				fmt.Printf("Error getting memo %d: %v, skipping\n", memo.ID, err)
				continue
			}

			// Create filename
			filename := fmt.Sprintf("%d_%s.%s", fullMemo.ID, sanitizeFilename(fullMemo.Title), format)
			filePath := filepath.Join(outputDir, filename)

			// Write file
			if err := writeMemoToFile(fullMemo, filePath, format); err != nil {
				fmt.Printf("Error writing memo %d to file: %v, skipping\n", fullMemo.ID, err)
				continue
			}

			fmt.Printf("Exported memo %d to %s\n", fullMemo.ID, filePath)
			count++

			// Sleep to avoid rate limiting
			time.Sleep(1 * time.Second)
		}

		if memoList.Meta.NextPage == nil {
			break
		}
		nextPage, _ := strconv.Atoi(*memoList.Meta.NextPage)
		page = nextPage
	}

	fmt.Println(color.GreenString("Export completed successfully"))
	fmt.Printf("Exported %d memos to %s\n", count, outputDir)
	return nil
}

// sanitizeFilename sanitizes a string to be used as a filename (rune-aware for multibyte characters)
func sanitizeFilename(s string) string {
	// Replace invalid characters with underscore
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Limit length (rune-aware)
	runes := []rune(result)
	if len(runes) > 50 {
		result = string(runes[:50])
	}
	return result
}

type memoFrontmatter struct {
	ID        int      `yaml:"id"`
	Title     string   `yaml:"title"`
	Author    string   `yaml:"author"`
	CreatedAt string   `yaml:"created_at"`
	UpdatedAt string   `yaml:"updated_at"`
	URL       string   `yaml:"url"`
	Draft     bool     `yaml:"draft"`
	Archived  bool     `yaml:"archived"`
	Scope     string   `yaml:"scope"`
	Tags      []string `yaml:"tags"`
	Groups    []string `yaml:"groups"`
}

// writeMemoToFile writes a memo to a file in the specified format
func writeMemoToFile(memo *docbase.Memo, filepath string, format string) error {
	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	switch format {
	case "md":
		tagNames := make([]string, 0, len(memo.Tags))
		for _, tag := range memo.Tags {
			tagNames = append(tagNames, tag.Name)
		}

		groupNames := make([]string, 0, len(memo.Groups))
		for _, grp := range memo.Groups {
			groupNames = append(groupNames, grp.Name)
		}

		frontmatter := memoFrontmatter{
			ID:        memo.ID,
			Title:     memo.Title,
			Author:    memo.User.Name,
			CreatedAt: memo.CreatedAt.Format(time.RFC3339),
			UpdatedAt: memo.UpdatedAt.Format(time.RFC3339),
			URL:       memo.URL,
			Draft:     memo.Draft,
			Archived:  memo.Archived,
			Scope:     memo.Scope,
			Tags:      tagNames,
			Groups:    groupNames,
		}

		frontmatterBytes, err := yaml.Marshal(frontmatter)
		if err != nil {
			return fmt.Errorf("failed to marshal frontmatter: %w", err)
		}

		if _, err := fmt.Fprintln(file, "---"); err != nil {
			return err
		}
		if _, err := file.Write(frontmatterBytes); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(file, "---"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(file); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(file, memo.Body); err != nil {
			return err
		}

	case "json":
		// Use json.Marshal to convert memo to JSON
		bytes, err := json.MarshalIndent(memo, "", "  ")
		if err != nil {
			return err
		}
		bytes = append(bytes, '\n')
		if _, err := file.Write(bytes); err != nil {
			return err
		}

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

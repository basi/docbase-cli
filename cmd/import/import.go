package import_cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/utils"
	"github.com/basi/docbase-cli/pkg/docbase"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Note: We use import_cmd as the package name because "import" is a reserved keyword in Go

var (
	// ImportCmd represents the import command
	ImportCmd = &cobra.Command{
		Use:   "import",
		Short: "Import memos",
		Long:  `Import memos to DocBase from local files.`,
	}

	// FileCmd represents the import file command
	FileCmd = &cobra.Command{
		Use:   "file [file_path]",
		Short: "Import a memo from a file",
		Long: `Import a memo to DocBase from a local file.

Example:
  docbase import file ./memo.md
  docbase import file ./memo.json --group "全員" --tag "週報"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			filePath := args[0]
			groupNames, _ := cmd.Flags().GetStringSlice("group")
			tagNames, _ := cmd.Flags().GetStringSlice("tag")
			draft, _ := cmd.Flags().GetBool("draft")
			scope, _ := cmd.Flags().GetString("scope")
			notify, _ := cmd.Flags().GetBool("notify")

			// Read file
			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			// Parse file
			var title, body string
			ext := strings.ToLower(filepath.Ext(filePath))

			switch ext {
			case ".md":
				title, body, err = parseMdFile(content)
				if err != nil {
					return err
				}
			case ".json":
				title, body, err = parseJsonFile(content)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported file format: %s", ext)
			}

			// Get group IDs
			var groupIDs []int
			if len(groupNames) > 0 {
				groups, err := client.Group.List(1, 100)
				if err != nil {
					return err
				}

				groupMap := make(map[string]int)
				for _, group := range groups.Groups {
					groupMap[group.Name] = group.ID
				}

				for _, name := range groupNames {
					id, ok := groupMap[name]
					if !ok {
						return fmt.Errorf("group not found: %s", name)
					}
					groupIDs = append(groupIDs, id)
				}
			}

			// Create memo
			req := &docbase.CreateMemoRequest{
				Title:  title,
				Body:   body,
				Draft:  draft,
				Tags:   tagNames,
				Scope:  scope,
				Groups: groupIDs,
				Notify: notify,
			}

			memo, err := client.Memo.Create(req)
			if err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo imported successfully"))
			fmt.Printf("ID: %d\n", memo.ID)
			fmt.Printf("URL: %s\n", memo.URL)

			return nil
		},
	}

	// DirCmd represents the import dir command
	DirCmd = &cobra.Command{
		Use:   "dir [dir_path]",
		Short: "Import memos from a directory",
		Long: `Import memos to DocBase from a directory.

Example:
  docbase import dir ./exports
  docbase import dir ./exports --group "全員" --tag "週報"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := utils.CreateClient(cmd)
			if err != nil {
				return err
			}

			dirPath := args[0]
			groupNames, _ := cmd.Flags().GetStringSlice("group")
			tagNames, _ := cmd.Flags().GetStringSlice("tag")
			draft, _ := cmd.Flags().GetBool("draft")
			scope, _ := cmd.Flags().GetString("scope")
			notify, _ := cmd.Flags().GetBool("notify")
			limit, _ := cmd.Flags().GetInt("limit")

			// Get group IDs
			var groupIDs []int
			if len(groupNames) > 0 {
				groups, err := client.Group.List(1, 100)
				if err != nil {
					return err
				}

				groupMap := make(map[string]int)
				for _, group := range groups.Groups {
					groupMap[group.Name] = group.ID
				}

				for _, name := range groupNames {
					id, ok := groupMap[name]
					if !ok {
						return fmt.Errorf("group not found: %s", name)
					}
					groupIDs = append(groupIDs, id)
				}
			}

			// List files in directory
			files, err := ioutil.ReadDir(dirPath)
			if err != nil {
				return fmt.Errorf("failed to read directory: %w", err)
			}

			// Filter files by extension
			var validFiles []string
			for _, file := range files {
				if file.IsDir() {
					continue
				}

				ext := strings.ToLower(filepath.Ext(file.Name()))
				if ext == ".md" || ext == ".json" {
					validFiles = append(validFiles, filepath.Join(dirPath, file.Name()))
				}
			}

			fmt.Printf("Found %d valid files\n", len(validFiles))

			// Import files
			count := 0
			for _, filePath := range validFiles {
				if limit > 0 && count >= limit {
					fmt.Println("Reached limit, stopping import")
					break
				}

				fmt.Printf("Importing %s...\n", filePath)

				// Read file
				content, err := ioutil.ReadFile(filePath)
				if err != nil {
					fmt.Printf("Error reading file %s: %v, skipping\n", filePath, err)
					continue
				}

				// Parse file
				var title, body string
				ext := strings.ToLower(filepath.Ext(filePath))

				switch ext {
				case ".md":
					title, body, err = parseMdFile(content)
					if err != nil {
						fmt.Printf("Error parsing file %s: %v, skipping\n", filePath, err)
						continue
					}
				case ".json":
					title, body, err = parseJsonFile(content)
					if err != nil {
						fmt.Printf("Error parsing file %s: %v, skipping\n", filePath, err)
						continue
					}
				default:
					fmt.Printf("Unsupported file format: %s, skipping\n", ext)
					continue
				}

				// Create memo
				req := &docbase.CreateMemoRequest{
					Title:  title,
					Body:   body,
					Draft:  draft,
					Tags:   tagNames,
					Scope:  scope,
					Groups: groupIDs,
					Notify: notify,
				}

				memo, err := client.Memo.Create(req)
				if err != nil {
					fmt.Printf("Error creating memo from file %s: %v, skipping\n", filePath, err)
					continue
				}

				fmt.Printf("Imported memo ID: %d, URL: %s\n", memo.ID, memo.URL)
				count++
			}

			fmt.Println(color.GreenString("Import completed successfully"))
			fmt.Printf("Imported %d memos\n", count)

			return nil
		},
	}
)

// parseMdFile parses a Markdown file with frontmatter
func parseMdFile(content []byte) (string, string, error) {
	contentStr := string(content)

	// Check if the file has frontmatter
	if !strings.HasPrefix(contentStr, "---\n") {
		// No frontmatter, use filename as title
		return "Imported Memo", contentStr, nil
	}

	// Find the end of frontmatter
	endIdx := strings.Index(contentStr[4:], "---\n")
	if endIdx == -1 {
		return "", "", fmt.Errorf("invalid frontmatter format")
	}
	endIdx += 4 // Adjust for the offset in the substring

	// Extract frontmatter and body
	frontmatter := contentStr[4:endIdx]
	body := contentStr[endIdx+4:]

	// Parse frontmatter
	var metadata map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return "", "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Extract title
	title, ok := metadata["title"].(string)
	if !ok || title == "" {
		title = "Imported Memo"
	}

	return title, body, nil
}

// parseJsonFile parses a JSON file
func parseJsonFile(content []byte) (string, string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return "", "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract title and body
	title, ok := data["title"].(string)
	if !ok || title == "" {
		title = "Imported Memo"
	}

	body, ok := data["body"].(string)
	if !ok || body == "" {
		return "", "", fmt.Errorf("body not found in JSON")
	}

	return title, body, nil
}

func init() {
	// Add import command to root command
	root.AddCommand(ImportCmd)

	// Add subcommands to import command
	ImportCmd.AddCommand(FileCmd)
	ImportCmd.AddCommand(DirCmd)

	// Add flags to file command
	FileCmd.Flags().StringSlice("group", []string{}, "Group names (can be specified multiple times)")
	FileCmd.Flags().StringSlice("tag", []string{}, "Tags (can be specified multiple times)")
	FileCmd.Flags().Bool("draft", false, "Save as draft")
	FileCmd.Flags().String("scope", "group", "Memo scope (group, private)")
	FileCmd.Flags().Bool("notify", false, "Send notification")

	// Add flags to dir command
	DirCmd.Flags().StringSlice("group", []string{}, "Group names (can be specified multiple times)")
	DirCmd.Flags().StringSlice("tag", []string{}, "Tags (can be specified multiple times)")
	DirCmd.Flags().Bool("draft", false, "Save as draft")
	DirCmd.Flags().String("scope", "group", "Memo scope (group, private)")
	DirCmd.Flags().Bool("notify", false, "Send notification")
	DirCmd.Flags().Int("limit", 0, "Limit the number of memos to import (0 for no limit)")
}
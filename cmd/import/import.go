package import_cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
			opts, err := getImportOptions(cmd)
			if err != nil {
				return err
			}

			// Read file
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			data, err := parseImportFile(filePath, content)
			if err != nil {
				return err
			}

			var groupMap map[string]int
			if opts.groupNamesChanged && len(opts.groupNames) > 0 {
				groupMap, err = utils.BuildGroupNameToIDMap(client)
				if err != nil {
					return err
				}
				opts.fixedGroupIDs, err = utils.ResolveGroupIDsFromMap(groupMap, dedupeStrings(normalizeStringSlice(opts.groupNames)))
				if err != nil {
					return err
				}
			}

			req, _, err := buildCreateMemoRequest(client, opts, data, groupMap)
			if err != nil {
				return err
			}

			// Create memo
			memo, err := client.Memo.Create(req)
			if err != nil {
				return err
			}

			if data.Archived != nil && *data.Archived {
				if err := client.Memo.Archive(memo.ID); err != nil {
					return err
				}
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
			opts, err := getImportOptions(cmd)
			if err != nil {
				return err
			}
			limit, _ := cmd.Flags().GetInt("limit")

			// List files in directory
			files, err := os.ReadDir(dirPath)
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

			var groupMap map[string]int
			if opts.groupNamesChanged && len(opts.groupNames) > 0 {
				groupMap, err = utils.BuildGroupNameToIDMap(client)
				if err != nil {
					return err
				}
				opts.fixedGroupIDs, err = utils.ResolveGroupIDsFromMap(groupMap, dedupeStrings(normalizeStringSlice(opts.groupNames)))
				if err != nil {
					return err
				}
			}

			// Import files
			count := 0
			for _, filePath := range validFiles {
				if limit > 0 && count >= limit {
					fmt.Println("Reached limit, stopping import")
					break
				}

				fmt.Printf("Importing %s...\n", filePath)

				// Read file
				content, err := os.ReadFile(filePath)
				if err != nil {
					fmt.Printf("Error reading file %s: %v, skipping\n", filePath, err)
					continue
				}

				data, err := parseImportFile(filePath, content)
				if err != nil {
					fmt.Printf("Error parsing file %s: %v, skipping\n", filePath, err)
					continue
				}

				var req *docbase.CreateMemoRequest
				req, groupMap, err = buildCreateMemoRequest(client, opts, data, groupMap)
				if err != nil {
					fmt.Printf("Error building request for file %s: %v, skipping\n", filePath, err)
					continue
				}

				memo, err := client.Memo.Create(req)
				if err != nil {
					fmt.Printf("Error creating memo from file %s: %v, skipping\n", filePath, err)
					continue
				}

				if data.Archived != nil && *data.Archived {
					if err := client.Memo.Archive(memo.ID); err != nil {
						fmt.Printf("Error archiving memo from file %s: %v, skipping\n", filePath, err)
						continue
					}
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

type importOptions struct {
	overwrite bool

	groupNames        []string
	groupNamesChanged bool
	fixedGroupIDs     []int

	tagNames        []string
	tagNamesChanged bool

	draft        bool
	draftChanged bool

	scope        string
	scopeChanged bool

	notify        bool
	notifyChanged bool
}

func getImportOptions(cmd *cobra.Command) (*importOptions, error) {
	groupNames, _ := cmd.Flags().GetStringSlice("group")
	tagNames, _ := cmd.Flags().GetStringSlice("tag")
	draft, _ := cmd.Flags().GetBool("draft")
	scope, _ := cmd.Flags().GetString("scope")
	notify, _ := cmd.Flags().GetBool("notify")
	overwrite, _ := cmd.Flags().GetBool("overwrite")

	return &importOptions{
		overwrite:         overwrite,
		groupNames:        groupNames,
		groupNamesChanged: cmd.Flags().Changed("group"),
		tagNames:          tagNames,
		tagNamesChanged:   cmd.Flags().Changed("tag"),
		draft:             draft,
		draftChanged:      cmd.Flags().Changed("draft"),
		scope:             scope,
		scopeChanged:      cmd.Flags().Changed("scope"),
		notify:            notify,
		notifyChanged:     cmd.Flags().Changed("notify"),
	}, nil
}

type importMemoData struct {
	Title    string
	Body     string
	Draft    *bool
	Archived *bool
	Scope    string
	Tags     []string
	Groups   []string
	GroupIDs []int
}

type markdownFrontmatter struct {
	ID       int      `yaml:"id"`
	Title    string   `yaml:"title"`
	Draft    *bool    `yaml:"draft"`
	Archived *bool    `yaml:"archived"`
	Scope    string   `yaml:"scope"`
	Tags     []string `yaml:"tags"`
	Groups   []string `yaml:"groups"`
}

func buildCreateMemoRequest(client *docbase.API, opts *importOptions, data *importMemoData, groupMap map[string]int) (*docbase.CreateMemoRequest, map[string]int, error) {
	title := strings.TrimSpace(data.Title)
	if title == "" {
		title = "Imported Memo"
	}

	tags := normalizeStringSlice(data.Tags)
	if opts.tagNamesChanged {
		flagTags := normalizeStringSlice(opts.tagNames)
		if opts.overwrite {
			tags = flagTags
		} else {
			tags = append(tags, flagTags...)
		}
	}
	tags = dedupeStrings(tags)

	var fileGroupIDs []int
	if !(opts.overwrite && opts.groupNamesChanged) {
		groupNames := dedupeStrings(normalizeStringSlice(data.Groups))
		if len(groupNames) > 0 {
			if groupMap == nil {
				var err error
				groupMap, err = utils.BuildGroupNameToIDMap(client)
				if err != nil {
					if len(data.GroupIDs) > 0 {
						fileGroupIDs = data.GroupIDs
					} else {
						return nil, groupMap, err
					}
				}
			}

			if fileGroupIDs == nil {
				var err error
				fileGroupIDs, err = utils.ResolveGroupIDsFromMap(groupMap, groupNames)
				if err != nil {
					if len(data.GroupIDs) > 0 {
						fileGroupIDs = data.GroupIDs
					} else {
						return nil, groupMap, err
					}
				}
			}
		} else if len(data.GroupIDs) > 0 {
			fileGroupIDs = data.GroupIDs
		}
	}

	var groupIDs []int
	if opts.groupNamesChanged {
		if opts.overwrite {
			groupIDs = opts.fixedGroupIDs
		} else {
			groupIDs = append(fileGroupIDs, opts.fixedGroupIDs...)
		}
	} else {
		groupIDs = fileGroupIDs
	}
	groupIDs = dedupeInts(groupIDs)

	draft := opts.draft
	if !opts.draftChanged && data.Draft != nil {
		draft = *data.Draft
	}

	scope := opts.scope
	if !opts.scopeChanged && strings.TrimSpace(data.Scope) != "" {
		scope = strings.TrimSpace(data.Scope)
	}

	notify := opts.notify

	return &docbase.CreateMemoRequest{
		Title:  title,
		Body:   data.Body,
		Draft:  draft,
		Tags:   tags,
		Scope:  scope,
		Groups: groupIDs,
		Notify: notify,
	}, groupMap, nil
}

func normalizeStringSlice(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func dedupeStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func dedupeInts(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func parseImportFile(filePath string, content []byte) (*importMemoData, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".md":
		return parseMdFile(content)
	case ".json":
		return parseJsonFile(content)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
}

// parseMdFile parses a Markdown file with frontmatter
func parseMdFile(content []byte) (*importMemoData, error) {
	contentStr := string(content)

	// Check if the file has frontmatter
	if !strings.HasPrefix(contentStr, "---\n") && !strings.HasPrefix(contentStr, "---\r\n") {
		// No frontmatter
		return &importMemoData{
			Title: "Imported Memo",
			Body:  contentStr,
		}, nil
	}

	// Skip the opening delimiter line (---)
	openingNewlineIdx := strings.IndexByte(contentStr, '\n')
	if openingNewlineIdx == -1 {
		return nil, fmt.Errorf("invalid frontmatter format: missing newline after opening delimiter")
	}

	frontmatterStart := openingNewlineIdx + 1
	pos := frontmatterStart

	for {
		if pos >= len(contentStr) {
			return nil, fmt.Errorf("invalid frontmatter format: missing closing delimiter")
		}

		lineEnd := strings.IndexByte(contentStr[pos:], '\n')
		var line string
		var nextPos int
		if lineEnd == -1 {
			line = contentStr[pos:]
			nextPos = len(contentStr)
		} else {
			line = contentStr[pos : pos+lineEnd]
			nextPos = pos + lineEnd + 1
		}

		line = strings.TrimSuffix(line, "\r")
		if line == "---" {
			frontmatterRaw := contentStr[frontmatterStart:pos]
			body := contentStr[nextPos:]

			frontmatter := strings.ReplaceAll(frontmatterRaw, "\r\n", "\n")

			// Parse frontmatter
			var metadata markdownFrontmatter
			if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			if after, ok := strings.CutPrefix(body, "\r\n"); ok {
				body = after
			} else if after, ok := strings.CutPrefix(body, "\n"); ok {
				body = after
			}

			title := strings.TrimSpace(metadata.Title)
			if title == "" {
				title = "Imported Memo"
			}

			return &importMemoData{
				Title:    title,
				Body:     body,
				Draft:    metadata.Draft,
				Archived: metadata.Archived,
				Scope:    metadata.Scope,
				Tags:     metadata.Tags,
				Groups:   metadata.Groups,
			}, nil
		}

		pos = nextPos
	}
}

// parseJsonFile parses a JSON file
func parseJsonFile(content []byte) (*importMemoData, error) {
	var memoResp docbase.MemoResponse
	if err := json.Unmarshal(content, &memoResp); err == nil && memoResp.Memo.ID != 0 {
		return memoToImportMemoData(&memoResp.Memo), nil
	}

	var memo docbase.Memo
	if err := json.Unmarshal(content, &memo); err == nil && memo.ID != 0 {
		return memoToImportMemoData(&memo), nil
	}

	type createMemoRequestJSON struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Draft  *bool    `json:"draft"`
		Tags   []string `json:"tags"`
		Scope  string   `json:"scope"`
		Groups []int    `json:"groups"`
	}
	var createReq createMemoRequestJSON
	if err := json.Unmarshal(content, &createReq); err == nil && createReq.Body != "" {
		title := strings.TrimSpace(createReq.Title)
		if title == "" {
			title = "Imported Memo"
		}
		return &importMemoData{
			Title:    title,
			Body:     createReq.Body,
			Draft:    createReq.Draft,
			Scope:    createReq.Scope,
			Tags:     createReq.Tags,
			GroupIDs: createReq.Groups,
		}, nil
	}

	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	title, ok := data["title"].(string)
	if !ok || strings.TrimSpace(title) == "" {
		title = "Imported Memo"
	}

	body, ok := data["body"].(string)
	if !ok || body == "" {
		return nil, fmt.Errorf("body not found in JSON")
	}

	return &importMemoData{
		Title: title,
		Body:  body,
	}, nil
}

func memoToImportMemoData(memo *docbase.Memo) *importMemoData {
	tags := make([]string, 0, len(memo.Tags))
	for _, tag := range memo.Tags {
		tags = append(tags, tag.Name)
	}

	groups := make([]string, 0, len(memo.Groups))
	groupIDs := make([]int, 0, len(memo.Groups))
	for _, group := range memo.Groups {
		groups = append(groups, group.Name)
		groupIDs = append(groupIDs, group.ID)
	}

	draft := memo.Draft
	archived := memo.Archived

	return &importMemoData{
		Title:    memo.Title,
		Body:     memo.Body,
		Draft:    &draft,
		Archived: &archived,
		Scope:    memo.Scope,
		Tags:     tags,
		Groups:   groups,
		GroupIDs: groupIDs,
	}
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
	FileCmd.Flags().Bool("overwrite", false, "Overwrite tags/groups from file with --tag/--group (default: merge)")
	FileCmd.Flags().Bool("draft", false, "Save as draft")
	FileCmd.Flags().String("scope", "group", "Memo scope (group, private)")
	FileCmd.Flags().Bool("notify", false, "Send notification")

	// Add flags to dir command
	DirCmd.Flags().StringSlice("group", []string{}, "Group names (can be specified multiple times)")
	DirCmd.Flags().StringSlice("tag", []string{}, "Tags (can be specified multiple times)")
	DirCmd.Flags().Bool("overwrite", false, "Overwrite tags/groups from file with --tag/--group (default: merge)")
	DirCmd.Flags().Bool("draft", false, "Save as draft")
	DirCmd.Flags().String("scope", "group", "Memo scope (group, private)")
	DirCmd.Flags().Bool("notify", false, "Send notification")
	DirCmd.Flags().Int("limit", 0, "Limit the number of memos to import (0 for no limit)")
}

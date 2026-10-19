package memo

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/internal/fileio"
	"github.com/basi/docbase-cli/internal/format"
	"github.com/basi/docbase-cli/internal/formatter"
	"github.com/basi/docbase-cli/internal/groups"
	"github.com/basi/docbase-cli/pkg/docbase"
)

var (
	// MemoCmd represents the memo command
	MemoCmd = &cobra.Command{
		Use:   "memo",
		Short: "Manage memos",
		Long:  `Manage memos in DocBase.`,
	}

	// ListCmd represents the memo list command
	ListCmd = &cobra.Command{
		Use:   "list",
		Short: "List memos",
		Long: `List memos in DocBase.

Example:
  docbase memo list
  docbase memo list --page 2 --per-page 20
  docbase memo list --query "tag:週報"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")
			query, _ := cmd.Flags().GetString("query")

			memoList, err := c.Memo.List(page, perPage, query)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for list
				fmt.Printf("Total: %d\n", memoList.Meta.Total)
				fmt.Println(strings.Repeat("-", 80))
				fmt.Printf("%-8s %-40s %-20s %s\n", "ID", "Title", "Author", "Tags")
				fmt.Println(strings.Repeat("-", 80))

				for _, memo := range memoList.Memos {
					fmt.Printf("%-8d %-40s %-20s %s\n",
						memo.ID,
						format.Truncate(memo.Title, 37),
						format.Truncate(memo.User.Name, 17),
						format.Truncate(format.Tags(memo.Tags), 20),
					)
				}

				if memoList.Meta.NextPage != nil {
					nextPage, _ := strconv.Atoi(*memoList.Meta.NextPage)
					fmt.Printf("\nUse --page %d to see the next page\n", nextPage)
				}
				return nil
			}

			return f.Print(memoList)
		},
	}

	// ViewCmd represents the memo view command
	ViewCmd = &cobra.Command{
		Use:   "view [id]",
		Short: "View a memo",
		Long: `View a memo in DocBase.

Example:
  docbase memo view 12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			memo, err := c.Memo.Get(id)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for view
				fmt.Printf("ID: %d\n", memo.ID)
				fmt.Printf("Title: %s\n", memo.Title)
				fmt.Printf("Author: %s\n", memo.User.Name)
				fmt.Printf("Created: %s\n", memo.CreatedAt.Format("2006-01-02 15:04:05"))
				fmt.Printf("Updated: %s\n", memo.UpdatedAt.Format("2006-01-02 15:04:05"))
				fmt.Printf("Tags: %s\n", format.Tags(memo.Tags))
				fmt.Printf("Groups: %s\n", format.Groups(memo.Groups))
				fmt.Printf("URL: %s\n", memo.URL)
				fmt.Println(strings.Repeat("-", 80))
				fmt.Println(memo.Body)
				return nil
			}

			return f.Print(memo)
		},
	}

	// CreateCmd represents the memo create command
	CreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a memo",
		Long: `Create a memo in DocBase.

Example:
  docbase memo create --title "Test Memo" --body "This is a test memo" --group "全員"
  docbase memo create --title "Test Memo" --body-file memo.md --tag "週報" --tag "開発"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			title, _ := cmd.Flags().GetString("title")
			body, _ := cmd.Flags().GetString("body")
			bodyFile, _ := cmd.Flags().GetString("body-file")
			draft, _ := cmd.Flags().GetBool("draft")
			scope, _ := cmd.Flags().GetString("scope")
			groupNames, _ := cmd.Flags().GetStringSlice("group")
			tagNames, _ := cmd.Flags().GetStringSlice("tag")
			notify, _ := cmd.Flags().GetBool("notify")
			excludeBody, _ := cmd.Flags().GetBool("exclude-body")

			if title == "" {
				return fmt.Errorf("title is required")
			}

			if body == "" && bodyFile == "" {
				return fmt.Errorf("either body or body-file is required")
			}

			if body == "" && bodyFile != "" {
				var err error
				body, err = fileio.Read(bodyFile)
				if err != nil {
					return err
				}
			}

			// Get group IDs
			groupIDs, err := groups.ResolveIDs(c, groupNames)
			if err != nil {
				return err
			}

			req := &docbase.CreateMemoRequest{
				Title:       title,
				Body:        body,
				Draft:       draft,
				Tags:        tagNames,
				Scope:       scope,
				Groups:      groupIDs,
				Notify:      notify,
				ExcludeBody: excludeBody,
			}

			memo, err := c.Memo.Create(req)
			if err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo created successfully"))
			fmt.Printf("ID: %d\n", memo.ID)
			fmt.Printf("URL: %s\n", memo.URL)

			return nil
		},
	}

	// EditCmd represents the memo edit command
	EditCmd = &cobra.Command{
		Use:   "edit [id]",
		Short: "Edit a memo",
		Long: `Edit a memo in DocBase.

Example:
  docbase memo edit 12345 --title "Updated Title"
  docbase memo edit 12345 --body-file updated.md --tag "週報" --tag "開発"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			title, _ := cmd.Flags().GetString("title")
			body, _ := cmd.Flags().GetString("body")
			bodyFile, _ := cmd.Flags().GetString("body-file")
			draft, _ := cmd.Flags().GetBool("draft")
			draftChanged := cmd.Flags().Changed("draft")
			scope, _ := cmd.Flags().GetString("scope")
			groupNames, _ := cmd.Flags().GetStringSlice("group")
			tagNames, _ := cmd.Flags().GetStringSlice("tag")
			notify, _ := cmd.Flags().GetBool("notify")
			notifyChanged := cmd.Flags().Changed("notify")
			excludeBody, _ := cmd.Flags().GetBool("exclude-body")

			if body == "" && bodyFile != "" {
				var err error
				body, err = fileio.Read(bodyFile)
				if err != nil {
					return err
				}
			}

			// Get group IDs
			groupIDs, err := groups.ResolveIDs(c, groupNames)
			if err != nil {
				return err
			}

			var draftPtr *bool
			if draftChanged {
				draftPtr = &draft
			}
			var notifyPtr *bool
			if notifyChanged {
				notifyPtr = &notify
			}

			req := &docbase.UpdateMemoRequest{
				Title:       title,
				Body:        body,
				Draft:       draftPtr,
				Tags:        tagNames,
				Scope:       scope,
				Groups:      groupIDs,
				Notify:      notifyPtr,
				ExcludeBody: excludeBody,
			}

			memo, err := c.Memo.Update(id, req)
			if err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo updated successfully"))
			fmt.Printf("ID: %d\n", memo.ID)
			fmt.Printf("URL: %s\n", memo.URL)

			return nil
		},
	}

	// DeleteCmd represents the memo delete command
	// Note: For safety, this command does not actually delete the memo.
	// Instead, it prepends [DELETE] to the title.
	DeleteCmd = &cobra.Command{
		Use:   "delete [id]",
		Short: "Mark a memo for deletion (adds [DELETE] prefix to title)",
		Long: `Mark a memo for deletion in DocBase.

For safety, this command does not actually delete the memo.
Instead, it prepends [DELETE] to the title.

Example:
  docbase memo delete 12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			// First, get the current memo to check the title
			memo, err := c.Memo.Get(id)
			if err != nil {
				return err
			}

			// Check if already marked for deletion
			if strings.HasPrefix(memo.Title, "[DELETE]") {
				fmt.Println(color.YellowString("Memo is already marked for deletion"))
				return nil
			}

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				fmt.Printf("Are you sure you want to mark memo %d for deletion? (y/N): ", id)
				var confirm string
				_, _ = fmt.Scanln(&confirm)
				if strings.ToLower(confirm) != "y" {
					fmt.Println("Operation canceled")
					return nil
				}
			}

			// Update the title with [DELETE] prefix and add delete tag
			newTitle := "[DELETE] " + memo.Title

			// Extract existing tag names and add "delete" tag
			var tags []string
			for _, tag := range memo.Tags {
				tags = append(tags, tag.Name)
			}
			// Add delete tag if not already present
			hasDeleteTag := slices.Contains(tags, "delete")
			if !hasDeleteTag {
				tags = append(tags, "delete")
			}

			req := &docbase.UpdateMemoRequest{
				Title: newTitle,
				Tags:  tags,
			}

			if _, err := c.Memo.Update(id, req); err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo marked for deletion (title updated with [DELETE] prefix, 'delete' tag added)"))
			fmt.Printf("URL: %s\n", memo.URL)
			return nil
		},
	}

	// ArchiveCmd represents the memo archive command
	ArchiveCmd = &cobra.Command{
		Use:   "archive [id]",
		Short: "Archive a memo",
		Long: `Archive a memo in DocBase.

Example:
  docbase memo archive 12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			if err := c.Memo.Archive(id); err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo archived successfully"))
			return nil
		},
	}

	// UnarchiveCmd represents the memo unarchive command
	UnarchiveCmd = &cobra.Command{
		Use:   "unarchive [id]",
		Short: "Unarchive a memo",
		Long: `Unarchive a memo in DocBase.

Example:
  docbase memo unarchive 12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			if err := c.Memo.Unarchive(id); err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo unarchived successfully"))
			return nil
		},
	}

	// SearchCmd represents the memo search command
	SearchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search memos",
		Long: `Search memos in DocBase.

Example:
  docbase memo search "keyword"
  docbase memo search "tag:週報 author:john"
  docbase memo search "group:全員 created_at:2023-01-01~2023-12-31"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			var query string
			if len(args) > 0 {
				query = args[0]
			}

			// Additional search parameters
			author, _ := cmd.Flags().GetString("author")
			group, _ := cmd.Flags().GetString("group")
			tag, _ := cmd.Flags().GetString("tag")
			createdAfter, _ := cmd.Flags().GetString("created-after")
			createdBefore, _ := cmd.Flags().GetString("created-before")

			// Build query string
			queryParts := []string{}
			if query != "" {
				queryParts = append(queryParts, query)
			}
			if author != "" {
				queryParts = append(queryParts, fmt.Sprintf("author:%s", author))
			}
			if group != "" {
				queryParts = append(queryParts, fmt.Sprintf("group:%s", group))
			}
			if tag != "" {
				queryParts = append(queryParts, fmt.Sprintf("tag:%s", tag))
			}
			if createdAfter != "" && createdBefore != "" {
				queryParts = append(queryParts, fmt.Sprintf("created_at:%s~%s", createdAfter, createdBefore))
			} else if createdAfter != "" {
				queryParts = append(queryParts, fmt.Sprintf("created_at:%s~*", createdAfter))
			} else if createdBefore != "" {
				queryParts = append(queryParts, fmt.Sprintf("created_at:*~%s", createdBefore))
			}

			finalQuery := strings.Join(queryParts, " ")

			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")

			memoList, err := c.Memo.List(page, perPage, finalQuery)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for list
				fmt.Printf("Total: %d\n", memoList.Meta.Total)
				fmt.Println(strings.Repeat("-", 80))
				fmt.Printf("%-8s %-40s %-20s %s\n", "ID", "Title", "Author", "Tags")
				fmt.Println(strings.Repeat("-", 80))

				for _, memo := range memoList.Memos {
					fmt.Printf("%-8d %-40s %-20s %s\n",
						memo.ID,
						format.Truncate(memo.Title, 37),
						format.Truncate(memo.User.Name, 17),
						format.Truncate(format.Tags(memo.Tags), 20),
					)
				}

				if memoList.Meta.NextPage != nil {
					nextPage, _ := strconv.Atoi(*memoList.Meta.NextPage)
					fmt.Printf("\nUse --page %d to see the next page\n", nextPage)
				}
				return nil
			}

			return f.Print(memoList)
		},
	}
	// PatchBodyCmd represents the memo patch-body command
	PatchBodyCmd = &cobra.Command{
		Use:   "patch-body [id]",
		Short: "Partially update a memo body line-by-line",
		Long: `Partially update specific lines in a memo body.

Each operation targets a 1-indexed line range. old_content must match the
current text exactly — if it doesn't the API rejects the update, preventing
accidental overwrites.

--op accepts a JSON object (single operation) or a JSON array (multiple operations):

  docbase memo patch-body 12345 \
    --op '{"start":3,"end":3,"old_content":"old line","content":"new line"}'

  docbase memo patch-body 12345 \
    --op '[{"start":3,"end":5,"old_content":"old\r\nlines","content":"new"}]' \
    --include-body`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			opStr, _ := cmd.Flags().GetString("op")
			if opStr == "" {
				return fmt.Errorf("--op is required")
			}

			var ops []docbase.PatchBodyOperation
			raw := []byte(opStr)
			if err := json.Unmarshal(raw, &ops); err != nil {
				var single docbase.PatchBodyOperation
				if err2 := json.Unmarshal(raw, &single); err2 != nil {
					return fmt.Errorf("--op must be a JSON object or array: %w", err)
				}
				ops = []docbase.PatchBodyOperation{single}
			}

			if len(ops) == 0 {
				return fmt.Errorf("at least one operation required")
			}
			for i, op := range ops {
				if op.Start == 0 || op.End == 0 {
					return fmt.Errorf("op[%d]: start and end are 1-indexed and required", i)
				}
			}

			includeBody, _ := cmd.Flags().GetBool("include-body")
			notify, _ := cmd.Flags().GetBool("notify")
			notifyChanged := cmd.Flags().Changed("notify")

			var notifyPtr *bool
			if notifyChanged {
				notifyPtr = &notify
			}

			req := &docbase.PatchBodyRequest{
				Operations:  ops,
				Notice:      notifyPtr,
				IncludeBody: includeBody,
			}

			memo, err := c.Memo.PatchBody(id, req)
			if err != nil {
				return err
			}

			fmt.Println(color.GreenString("Memo body patched successfully"))
			fmt.Printf("ID: %d\n", memo.ID)
			if memo.URL != "" {
				fmt.Printf("URL: %s\n", memo.URL)
			}
			if includeBody && memo.Body != "" {
				fmt.Println(strings.Repeat("-", 80))
				fmt.Println(memo.Body)
			}

			return nil
		},
	}
)

func init() {
	// Add memo command to root command
	root.AddCommand(MemoCmd)

	// Add subcommands to memo command
	MemoCmd.AddCommand(ListCmd)
	MemoCmd.AddCommand(ViewCmd)
	MemoCmd.AddCommand(CreateCmd)
	MemoCmd.AddCommand(EditCmd)
	MemoCmd.AddCommand(DeleteCmd)
	MemoCmd.AddCommand(ArchiveCmd)
	MemoCmd.AddCommand(UnarchiveCmd)
	MemoCmd.AddCommand(SearchCmd)
	MemoCmd.AddCommand(PatchBodyCmd)

	// Add flags to list command
	ListCmd.Flags().Int("page", 1, "Page number")
	ListCmd.Flags().Int("per-page", 20, "Number of items per page")
	ListCmd.Flags().String("query", "", "Search query")

	// Add flags to create command
	CreateCmd.Flags().String("title", "", "Memo title")
	CreateCmd.Flags().String("body", "", "Memo body")
	CreateCmd.Flags().String("body-file", "", "File containing memo body")
	CreateCmd.Flags().Bool("draft", false, "Save as draft")
	CreateCmd.Flags().String("scope", "group", "Memo scope (group, private)")
	CreateCmd.Flags().StringSlice("group", []string{}, "Group names (can be specified multiple times)")
	CreateCmd.Flags().StringSlice("tag", []string{}, "Tags (can be specified multiple times)")
	CreateCmd.Flags().Bool("notify", false, "Send notification")
	CreateCmd.Flags().Bool("exclude-body", false, "Omit body from response to reduce bandwidth")

	// Add flags to edit command
	EditCmd.Flags().String("title", "", "Memo title")
	EditCmd.Flags().String("body", "", "Memo body")
	EditCmd.Flags().String("body-file", "", "File containing memo body")
	EditCmd.Flags().Bool("draft", false, "Save as draft")
	EditCmd.Flags().String("scope", "", "Memo scope (group, private)")
	EditCmd.Flags().StringSlice("group", []string{}, "Group names (can be specified multiple times)")
	EditCmd.Flags().StringSlice("tag", []string{}, "Tags (can be specified multiple times)")
	EditCmd.Flags().Bool("notify", false, "Send notification")
	EditCmd.Flags().Bool("exclude-body", false, "Omit body from response to reduce bandwidth")

	// Add flags to patch-body command
	PatchBodyCmd.Flags().String("op", "", "Patch operation(s) as JSON object or array (required)")
	PatchBodyCmd.Flags().Bool("include-body", false, "Include updated body in response")
	PatchBodyCmd.Flags().Bool("notify", false, "Send notification (server default: true; specify --notify=false to disable)")
	_ = PatchBodyCmd.MarkFlagRequired("op")

	// Add flags to delete command
	DeleteCmd.Flags().Bool("force", false, "Force deletion without confirmation")

	// Add flags to search command
	SearchCmd.Flags().Int("page", 1, "Page number")
	SearchCmd.Flags().Int("per-page", 20, "Number of items per page")
	SearchCmd.Flags().String("author", "", "Filter by author")
	SearchCmd.Flags().String("group", "", "Filter by group")
	SearchCmd.Flags().String("tag", "", "Filter by tag")
	SearchCmd.Flags().String("created-after", "", "Filter by creation date (YYYY-MM-DD)")
	SearchCmd.Flags().String("created-before", "", "Filter by creation date (YYYY-MM-DD)")
}

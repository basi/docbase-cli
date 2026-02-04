package comment

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/internal/fileio"
	"github.com/basi/docbase-cli/internal/formatter"
	"github.com/basi/docbase-cli/pkg/docbase"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// CommentCmd represents the comment command
	CommentCmd = &cobra.Command{
		Use:   "comment",
		Short: "Manage comments",
		Long:  `Manage comments in DocBase.`,
	}

	// ListCmd represents the comment list command
	ListCmd = &cobra.Command{
		Use:   "list [memo_id]",
		Short: "List comments for a memo",
		Long: `List comments for a memo in DocBase.

Example:
  docbase comment list 12345
  docbase comment list 12345 --page 2 --per-page 20`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			memoID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			page, _ := cmd.Flags().GetInt("page")
			perPage, _ := cmd.Flags().GetInt("per-page")

			commentList, err := c.Comment.List(memoID, page, perPage)
			if err != nil {
				return err
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			if outputFormat == "text" {
				// Custom text format for list
				fmt.Printf("Memo ID: %d\n", memoID)
				fmt.Printf("Total Comments: %d\n", commentList.Meta.Total)
				fmt.Println(strings.Repeat("-", 80))

				for _, cmt := range commentList.Comments {
					fmt.Printf("ID: %d\n", cmt.ID)
					fmt.Printf("Author: %s\n", cmt.User.Name)
					fmt.Printf("Created At: %s\n", cmt.CreatedAt.Format("2006-01-02 15:04:05"))
					fmt.Println(strings.Repeat("-", 40))
					fmt.Println(cmt.Body)
					fmt.Println(strings.Repeat("-", 80))
				}

				if commentList.Meta.NextPage != nil {
					nextPage, _ := strconv.Atoi(*commentList.Meta.NextPage)
					fmt.Printf("\nUse --page %d to see the next page\n", nextPage)
				}
				return nil
			}

			return f.Print(commentList)
		},
	}

	// CreateCmd represents the comment create command
	CreateCmd = &cobra.Command{
		Use:   "create [memo_id]",
		Short: "Create a comment",
		Long: `Create a comment for a memo in DocBase.

Example:
  docbase comment create 12345 --body "This is a comment"
  docbase comment create 12345 --body-file comment.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			memoID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid memo ID: %s", args[0])
			}

			body, _ := cmd.Flags().GetString("body")
			bodyFile, _ := cmd.Flags().GetString("body-file")
			notify, _ := cmd.Flags().GetBool("notify")

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

			req := &docbase.CreateCommentRequest{
				Body:   body,
				Notify: notify,
			}

			cmt, err := c.Comment.Create(memoID, req)
			if err != nil {
				return err
			}

			fmt.Println(color.GreenString("Comment created successfully"))
			fmt.Printf("ID: %d\n", cmt.ID)
			return nil
		},
	}

	// DeleteCmd represents the comment delete command
	DeleteCmd = &cobra.Command{
		Use:   "delete [comment_id]",
		Short: "Delete a comment",
		Long: `Delete a comment from DocBase.

Example:
  docbase comment delete 67890`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			commentID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid comment ID: %s", args[0])
			}

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				fmt.Printf("Are you sure you want to delete comment %d? (y/N): ", commentID)
				var confirm string
				fmt.Scanln(&confirm)
				if strings.ToLower(confirm) != "y" {
					fmt.Println("Deletion cancelled")
					return nil
				}
			}

			if err := c.Comment.Delete(commentID); err != nil {
				return err
			}

			fmt.Println(color.GreenString("Comment deleted successfully"))
			return nil
		},
	}
)

func init() {
	// Add comment command to root command
	root.AddCommand(CommentCmd)

	// Add subcommands to comment command
	CommentCmd.AddCommand(ListCmd)
	CommentCmd.AddCommand(CreateCmd)
	CommentCmd.AddCommand(DeleteCmd)

	// Add flags to list command
	ListCmd.Flags().Int("page", 1, "Page number")
	ListCmd.Flags().Int("per-page", 20, "Number of items per page")

	// Add flags to create command
	CreateCmd.Flags().String("body", "", "Comment body")
	CreateCmd.Flags().String("body-file", "", "File containing comment body")
	CreateCmd.Flags().Bool("notify", false, "Send notification")

	// Add flags to delete command
	DeleteCmd.Flags().Bool("force", false, "Force deletion without confirmation")
}

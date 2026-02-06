package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/client"
	"github.com/basi/docbase-cli/internal/formatter"
)

var (
	// APICmd represents the api command
	APICmd = &cobra.Command{
		Use:   "api",
		Short: "Make direct API requests",
		Long: `Make direct API requests to DocBase API.

This command allows you to make direct API requests to DocBase API endpoints.
It's useful for accessing API endpoints that are not yet supported by the CLI.`,
	}

	// GetCmd represents the api get command
	GetCmd = &cobra.Command{
		Use:   "get [path]",
		Short: "Make a GET request",
		Long: `Make a GET request to DocBase API.

Example:
  docbase api get /posts
  docbase api get /posts/12345
  docbase api get /posts?q=tag:weekly`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			// Extract query parameters
			params := make(map[string]string)
			if strings.Contains(path, "?") {
				parts := strings.SplitN(path, "?", 2)
				path = parts[0]
				queryParams := strings.SplitSeq(parts[1], "&")
				for param := range queryParams {
					if strings.Contains(param, "=") {
						kv := strings.SplitN(param, "=", 2)
						params[kv[0]] = kv[1]
					}
				}
			}

			resp, err := c.Get(path, params)
			if err != nil {
				return err
			}

			if resp.IsError() {
				return fmt.Errorf("API error: %s", resp.Status())
			}

			// Parse response as JSON
			var data any
			if err := json.Unmarshal(resp.Body(), &data); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)
			return f.Print(data)
		},
	}

	// PostCmd represents the api post command
	PostCmd = &cobra.Command{
		Use:   "post [path]",
		Short: "Make a POST request",
		Long: `Make a POST request to DocBase API.

Example:
  docbase api post /posts --data '{"title":"Test","body":"Test body","draft":false,"tags":["test"],"scope":"group","groups":[1],"notice":false}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			data, _ := cmd.Flags().GetString("data")
			if data == "" {
				return fmt.Errorf("data is required")
			}

			// Parse data as JSON
			var jsonData any
			if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
				return fmt.Errorf("invalid JSON data: %w", err)
			}

			resp, err := c.Post(path, jsonData)
			if err != nil {
				return err
			}

			if resp.IsError() {
				return fmt.Errorf("API error: %s", resp.Status())
			}

			// Parse response as JSON
			var respData any
			if err := json.Unmarshal(resp.Body(), &respData); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)
			return f.Print(respData)
		},
	}

	// PutCmd represents the api put command
	PutCmd = &cobra.Command{
		Use:   "put [path]",
		Short: "Make a PUT request",
		Long: `Make a PUT request to DocBase API.

Example:
  docbase api put /posts/12345 --data '{"title":"Updated Title","body":"Updated body"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			data, _ := cmd.Flags().GetString("data")
			if data == "" {
				return fmt.Errorf("data is required")
			}

			// Parse data as JSON
			var jsonData any
			if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
				return fmt.Errorf("invalid JSON data: %w", err)
			}

			resp, err := c.Put(path, jsonData)
			if err != nil {
				return err
			}

			if resp.IsError() {
				return fmt.Errorf("API error: %s", resp.Status())
			}

			// Parse response as JSON
			var respData any
			if err := json.Unmarshal(resp.Body(), &respData); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)
			return f.Print(respData)
		},
	}

	// DeleteCmd represents the api delete command
	DeleteCmd = &cobra.Command{
		Use:   "delete [path]",
		Short: "Make a DELETE request",
		Long: `Make a DELETE request to DocBase API.

Example:
  docbase api delete /posts/12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.Create(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			resp, err := c.Delete(path)
			if err != nil {
				return err
			}

			if resp.IsError() {
				return fmt.Errorf("API error: %s", resp.Status())
			}

			fmt.Println("Request successful")
			return nil
		},
	}
)

func init() {
	// Add api command to root command
	root.AddCommand(APICmd)

	// Add subcommands to api command
	APICmd.AddCommand(GetCmd)
	APICmd.AddCommand(PostCmd)
	APICmd.AddCommand(PutCmd)
	APICmd.AddCommand(DeleteCmd)

	// Add flags to post command
	PostCmd.Flags().String("data", "", "JSON data for the request")
	_ = PostCmd.MarkFlagRequired("data")

	// Add flags to put command
	PutCmd.Flags().String("data", "", "JSON data for the request")
	_ = PutCmd.MarkFlagRequired("data")
}

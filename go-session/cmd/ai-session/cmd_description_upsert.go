// Copyright © 2024 The Homeport Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/daniel-talonone/gemini-commands/internal/description"
	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/spf13/cobra"
)

// descriptionUpsertCmd represents the upsert command
var descriptionUpsertCmd = &cobra.Command{
	Use:   "upsert <story-id> [<content>]",
	Short: "Create or update a description.md file for a feature",
	Long: `Create or update a description.md file for a feature.

The content can be provided as an argument or piped from stdin.
It is an error to provide both at the same time.
If the description.md file already exists, it will be overwritten.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyID := args[0]
		var content string

		info, err := os.Stdin.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat stdin: %w", err)
		}

		hasPipe := (info.Mode() & os.ModeCharDevice) == 0
		hasContentArg := len(args) == 2

		if hasPipe && hasContentArg {
			return fmt.Errorf("ambiguous input: both stdin and positional argument provided")
		}

		if !hasPipe && !hasContentArg {
			return fmt.Errorf("neither stdin nor positional argument provided")
		}

		if hasPipe {
			bytes, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			content = string(bytes)
		} else {
			content = args[1]
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}

		remoteURL := git.RemoteURL()

		featureDir, err := feature.ResolveFeatureDir(storyID, cwd, remoteURL)
		if err != nil {
			return fmt.Errorf("failed to resolve feature directory for story %s: %w", storyID, err)
		}

		if _, err := os.Stat(featureDir); os.IsNotExist(err) {
			return fmt.Errorf("feature directory not found: %s; use resolve-feature-dir to debug", featureDir)
		}

		if err := description.UpsertDescription(featureDir, content); err != nil {
			return err
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), "description.md written successfully.")
		return err
	},
}

func init() {
	descriptionCmd.AddCommand(descriptionUpsertCmd)
}

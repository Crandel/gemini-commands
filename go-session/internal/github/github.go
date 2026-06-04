package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

// New structs for GraphQL response
type gqlComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body string `json:"body"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

type gqlComments struct {
	Nodes []gqlComment `json:"nodes"`
}

type gqlThread struct {
	IsResolved bool        `json:"isResolved"`
	Comments   gqlComments `json:"comments"`
}

type gqlThreads struct {
	Nodes []gqlThread `json:"nodes"`
}

type gqlPullRequest struct {
	ReviewThreads gqlThreads `json:"reviewThreads"`
}

type gqlRepository struct {
	PullRequest gqlPullRequest `json:"pullRequest"`
}

type gqlResponse struct {
	Data struct {
		Repository gqlRepository `json:"repository"`
	} `json:"data"`
}

func GetUnresolvedReviewThreads(workDir, repo, branch string) (string, error) {
	// 1. Get PR number for the current branch
	prCmd := execCommand("gh", "pr", "list", "--head", branch, "--state", "open", "--json", "number", "--limit", "1")
	prCmd.Dir = workDir
	var prOut bytes.Buffer
	prCmd.Stdout = &prOut
	var prErr bytes.Buffer
	prCmd.Stderr = &prErr
	if err := prCmd.Run(); err != nil {
		return "", fmt.Errorf("getting PR number failed: %s: %w", prErr.String(), err)
	}

	var prs []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(prOut.Bytes(), &prs); err != nil {
		return "", fmt.Errorf("parsing PR number: %w", err)
	}
	if len(prs) == 0 {
		return "", nil // No open PR found for this branch
	}
	prNumber := prs[0].Number

	// 2. Get repo owner and name
	repoParts := strings.Split(repo, "/")
	if len(repoParts) != 2 {
		return "", fmt.Errorf("invalid repo format: %q", repo)
	}
	owner, repoName := repoParts[0], repoParts[1]

	// 3. Prepare and run GraphQL query
	query := `
	query($owner: String!, $repo: String!, $pr: Int!) {
	  repository(owner: $owner, name: $repo) {
	    pullRequest(number: $pr) {
	      reviewThreads(first: 100) {
	        nodes {
	          isResolved
	          comments(first: 10) {
	            nodes {
	              author {
	                login
	              }
	              body
	              path
	              line
	            }
	          }
	        }
	      }
	    }
	  }
	}`

	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\t", " ")

	apiCmd := execCommand("gh", "api", "graphql",
		"-f", fmt.Sprintf("query=%s", query),
		"-f", fmt.Sprintf("owner=%s", owner),
		"-f", fmt.Sprintf("repo=%s", repoName),
		"-F", fmt.Sprintf("pr=%d", prNumber),
	)
	apiCmd.Dir = workDir
	var apiOut bytes.Buffer
	var apiErr bytes.Buffer
	apiCmd.Stdout = &apiOut
	apiCmd.Stderr = &apiErr
	if err := apiCmd.Run(); err != nil {
		return "", fmt.Errorf("gh api graphql failed: %s: %w", apiErr.String(), err)
	}

	var resp gqlResponse
	if err := json.Unmarshal(apiOut.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parsing graphql response: %w", err)
	}

	var formattedThreads strings.Builder
	for _, thread := range resp.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if !thread.IsResolved {
			for _, comment := range thread.Comments.Nodes {
				var fileLine string
				if comment.Line == 0 {
					fileLine = comment.Path
				} else {
					fileLine = fmt.Sprintf("%s:%d", comment.Path, comment.Line)
				}
				fmt.Fprintf(&formattedThreads, `File: %s
Author: %s
Comment: %s

`,
					fileLine,
					comment.Author.Login,
					comment.Body)
			}
		}
	}

	return formattedThreads.String(), nil
}

// CreatePR creates a GitHub pull request and returns its URL.
// It first checks if a PR already exists for the given branch and returns an error if so.
func CreatePR(workDir, base, head, title, body string) (string, error) {
	return CreatePRImpl(workDir, base, head, title, body)
}

// createPRImpl is the actual implementation of CreatePR, allowing it to be swapped out for testing.
var CreatePRImpl = func(workDir, base, head, title, body string) (string, error) {
	cmdView := execCommand("gh", "pr", "view", head, "--json", "url,state")
	cmdView.Dir = workDir
	var outView bytes.Buffer
	var errView bytes.Buffer
	cmdView.Stdout = &outView
	cmdView.Stderr = &errView
	viewErr := cmdView.Run()

	if viewErr == nil {
		// A PR record exists — check its state.
		var prInfo struct {
			URL   string `json:"url"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(outView.Bytes(), &prInfo); err != nil {
			return "", fmt.Errorf("parsing existing PR info: %w", err)
		}
		if prInfo.State == "OPEN" {
			// PR is open — return the existing URL, no need to create.
			return prInfo.URL, nil
		}
		// PR is closed or merged — proceed to create a new one.
	} else if !strings.Contains(errView.String(), "no pull requests found") {
		// gh pr view failed for a reason other than "no PR found" — surface the error.
		return "", fmt.Errorf("failed to check for existing PR: %s: %w", errView.String(), viewErr)
	}

	// PR does not exist, create it.
	// Write the body to a temp file to avoid shell-length limits with multiline content.
	args := []string{"pr", "create", "--base", base, "--head", head, "--title", title}
	var tmpBodyFile string
	if body != "" {
		f, err := os.CreateTemp("", "pr-body-*.md")
		if err != nil {
			return "", fmt.Errorf("creating body temp file: %w", err)
		}
		tmpBodyFile = f.Name()
		defer os.Remove(tmpBodyFile) //nolint:errcheck
		if _, err := f.WriteString(body); err != nil {
			f.Close() //nolint:errcheck
			return "", fmt.Errorf("writing body temp file: %w", err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("closing body temp file: %w", err)
		}
		args = append(args, "--body-file", tmpBodyFile)
	}

	cmdCreate := execCommand("gh", args...)
	cmdCreate.Dir = workDir
	var outCreate bytes.Buffer
	var errCreate bytes.Buffer
	cmdCreate.Stdout = &outCreate
	cmdCreate.Stderr = &errCreate
	if err := cmdCreate.Run(); err != nil {
		return "", fmt.Errorf("gh pr create failed: %s: %w", errCreate.String(), err)
	}

	prURL := strings.TrimSpace(outCreate.String())
	return prURL, nil
}

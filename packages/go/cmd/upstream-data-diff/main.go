// Command upstream-data-diff diffs prices/data.schema.json and prices/data.json
// between two upstream (pydantic/genai-prices) release versions and renders a
// markdown report.
//
// It is run by .github/workflows/upstream-data-diff.yml on Dependabot
// upstream-release PRs, but works standalone:
//
//	go run ./cmd/upstream-data-diff <old-version> <new-version> [--pr <number>]
//
// Env:
//
//	UPSTREAM_REPO  upstream repo (default pydantic/genai-prices)
//	GH_TOKEN       used by gh; required to fetch files and to post a comment
//	OUT            optional path to also write the markdown report to
//
// With --pr it posts/updates a single sticky comment (matched by a hidden
// marker) on that PR in the *current* repo. Without it, the report is printed
// to stdout. A comment is always delivered, even when the run fails partway:
// the partial report plus a failure note is posted/printed and the process
// exits non-zero so the workflow still surfaces the error.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	upstreamRepo       = "pydantic/genai-prices"
	dataSchemaFileName = "data.schema.json"
)

func main() {
	old, newV, pr, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "usage: %s <old-version> <new-version> [--pr <number>]\n", os.Args[0])
		os.Exit(2)
	}

	report, buildErr := buildReport(old, newV)
	if buildErr != nil {
		report += fmt.Sprintf(
			"\n> ❌ The diff tool failed before completing — the report above may be partial.\n"+
				"> Error: %v\n", buildErr)
	}

	if err := deliver(report, pr); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if buildErr != nil {
		os.Exit(1)
	}
}

func parseArgs(args []string) (old, newV, pr string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pr":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--pr requires a value")
			}
			i++
			pr = args[i]
		case "-h", "--help":
			return "", "", "", fmt.Errorf("help requested")
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", fmt.Errorf("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 2 {
		return "", "", "", fmt.Errorf("expected <old-version> and <new-version>")
	}
	return positional[0], positional[1], pr, nil
}

// buildReport assembles the markdown report. A non-nil error means the report
// is incomplete (the caller still delivers what was built, with a note).
func buildReport(old, newV string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Comparing tags `v%s` and `v%s`.\n\n", old, newV)

	// data.schema.json — the Go-impl signal.
	fmt.Fprintln(&b, "### `prices/"+dataSchemaFileName+"`")
	fmt.Fprintln(&b)
	schemaOld, errOld := ghFetch(upstreamRepo, old, dataSchemaFileName)
	schemaNew, errNew := ghFetch(upstreamRepo, newV, dataSchemaFileName)
	switch {
	case errOld != nil || errNew != nil:
		fmt.Fprintf(&b, "> ⏳ Could not fetch the schema for one or both tags (the git tag may not be "+
			"pushed yet). Re-run once the upstream release is tagged.\n")
	default:
		diff, changed, derr := diffU(schemaOld, schemaNew,
			"v"+old+"/prices/data.schema.json", "v"+newV+"/prices/data.schema.json")
		if derr != nil {
			return b.String(), fmt.Errorf("diff data.schema.json: %w", derr)
		}
		if !changed {
			fmt.Fprintln(&b, "✅ **No schema change.**")
		} else {
			fmt.Fprintln(&b, "⚠️ **Schema changed — the Go implementation in `packages/go/` likely needs updating.**")
			fmt.Fprintln(&b)
			fmt.Fprintf(&b, "```diff\n%s```\n", ensureTrailingNewline(diff))
		}
	}
	fmt.Fprintln(&b)

	return b.String(), nil
}

// ghFetch returns prices/<file> at tag v<version> from repo. A non-nil error
// means the tag or file is missing (e.g. the tag is not pushed yet).
func ghFetch(repo, version, file string) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/contents/prices/%s?ref=v%s", repo, file, version)
	cmd := exec.Command("gh", "api", path, "-H", "Accept: application/vnd.github.raw")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh api %s: %v: %s", path, err, strings.TrimSpace(errBuf.String()))
	}
	return out.Bytes(), nil
}

// diffU runs `diff -u` over two byte slices written to temp files and returns
// the unified diff, whether they differ, and an error for any real failure.
// diff's exit codes: 0 = identical, 1 = differ, >1 = trouble.
func diffU(a, b []byte, labelA, labelB string) (string, bool, error) {
	fa, err := writeTemp(a)
	if err != nil {
		return "", false, err
	}
	defer os.Remove(fa)
	fb, err := writeTemp(b)
	if err != nil {
		return "", false, err
	}
	defer os.Remove(fb)

	cmd := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, fa, fb)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	if err == nil {
		return "", false, nil // identical
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return out.String(), true, nil // differ
	}
	return "", false, fmt.Errorf("diff: %v: %s", err, strings.TrimSpace(out.String()))
}

func writeTemp(data []byte) (string, error) {
	f, err := os.CreateTemp("", "upstream-data-diff-*")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// ---- delivery (stdout or sticky PR comment) -------------------------------

func deliver(report, pr string) error {
	if pr == "" {
		fmt.Print(report)
		return nil
	}
	return postComment(pr, report)
}

// postComment posts, or sticky-updates by marker, the report on PR in the
// current repo.
func postComment(pr, body string) error {
	id, err := existingCommentID(pr)
	if err != nil {
		return err
	}
	bodyFile, err := writeTemp([]byte(body))
	if err != nil {
		return err
	}
	defer os.Remove(bodyFile)

	var path, method string
	if id != "" {
		path = "repos/{owner}/{repo}/issues/comments/" + id
		method = "PATCH"
	} else {
		path = "repos/{owner}/{repo}/issues/" + pr + "/comments"
		method = "POST"
	}
	cmd := exec.Command("gh", "api", "-X", method, path, "-F", "body=@"+bodyFile)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("post comment to PR #%s: %v: %s", pr, err, strings.TrimSpace(errBuf.String()))
	}
	if id != "" {
		fmt.Printf("Updated comment %s on PR #%s.\n", id, pr)
	} else {
		fmt.Printf("Created comment on PR #%s.\n", pr)
	}
	return nil
}

func existingCommentID(pr string) (string, error) {
	jqExpr := fmt.Sprintf("[.[] | select(.body | contains(%q)) | .id] | first // empty", "Comparing tags")
	cmd := exec.Command("gh", "api", "repos/{owner}/{repo}/issues/"+pr+"/comments", "--paginate", "--jq", jqExpr)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("list comments on PR #%s: %v: %s", pr, err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

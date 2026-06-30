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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"

	genaiprices "github.com/honeycombio/genai-prices/packages/go"
)

const marker = "<!-- upstream-data-diff -->"

// listLimit caps how many entries each <details> list shows before collapsing
// the remainder into a "… N more" line.
const listLimit = 20

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

func upstreamRepo() string {
	if r := os.Getenv("UPSTREAM_REPO"); r != "" {
		return r
	}
	return "pydantic/genai-prices"
}

// buildReport assembles the markdown report. A non-nil error means the report
// is incomplete (the caller still delivers what was built, with a note).
func buildReport(old, newV string) (string, error) {
	repo := upstreamRepo()
	var b strings.Builder
	fmt.Fprintln(&b, marker)
	fmt.Fprintf(&b, "## Upstream data diff: `%s` → `%s`\n\n", old, newV)
	fmt.Fprintf(&b, "Comparing [`%s`](https://github.com/%s) tags `v%s` and `v%s`.\n\n", repo, repo, old, newV)

	// data.schema.json — the Go-impl signal.
	fmt.Fprintln(&b, "### `prices/data.schema.json`")
	fmt.Fprintln(&b)
	schemaOld, errOld := ghFetch(repo, old, "data.schema.json")
	schemaNew, errNew := ghFetch(repo, newV, "data.schema.json")
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

	// data.json — structural summary + collapsed full diff.
	fmt.Fprintln(&b, "### `prices/data.json`")
	fmt.Fprintln(&b)
	dataOld, errOld := ghFetch(repo, old, "data.json")
	dataNew, errNew := ghFetch(repo, newV, "data.json")
	if errOld != nil || errNew != nil {
		fmt.Fprintf(&b, "> ⏳ Could not fetch data.json for one or both tags (the git tag may not be "+
			"pushed yet). Re-run once the upstream release is tagged.\n")
		return b.String(), nil
	}
	if err := writeDataSection(&b, old, newV, dataOld, dataNew); err != nil {
		return b.String(), fmt.Errorf("diff data.json: %w", err)
	}
	return b.String(), nil
}

func writeDataSection(b *strings.Builder, old, newV string, dataOld, dataNew []byte) error {
	var provsOld, provsNew []genaiprices.Provider
	if err := json.Unmarshal(dataOld, &provsOld); err != nil {
		return fmt.Errorf("parse v%s: %w", old, err)
	}
	if err := json.Unmarshal(dataNew, &provsNew); err != nil {
		return fmt.Errorf("parse v%s: %w", newV, err)
	}

	d := diffCatalog(provsOld, provsNew)
	if d.empty() {
		fmt.Fprintln(b, "✅ **No data change.**")
		return nil
	}

	fmt.Fprintf(b, "Models: **+%d** added · **−%d** removed · **~%d** changed.\n",
		len(d.ModelsAdded), len(d.ModelsRemoved), len(d.ModelsChanged))
	if len(d.ProvidersAdded) > 0 || len(d.ProvidersRemoved) > 0 {
		fmt.Fprintf(b, "\nProviders: **+%d** added · **−%d** removed.\n",
			len(d.ProvidersAdded), len(d.ProvidersRemoved))
	}
	fmt.Fprintln(b)

	emitSection(b, "Providers added", d.ProvidersAdded)
	emitSection(b, "Providers removed", d.ProvidersRemoved)
	emitSection(b, "Models added", d.ModelsAdded)
	emitSection(b, "Models removed", d.ModelsRemoved)
	emitSection(b, "Models changed", d.ModelsChanged)

	// Full pretty-printed diff (the source is minified, so indent first).
	prettyOld, err := indentJSON(dataOld)
	if err != nil {
		return fmt.Errorf("indent v%s: %w", old, err)
	}
	prettyNew, err := indentJSON(dataNew)
	if err != nil {
		return fmt.Errorf("indent v%s: %w", newV, err)
	}
	diff, _, derr := diffU(prettyOld, prettyNew, "v"+old+"/prices/data.json", "v"+newV+"/prices/data.json")
	if derr != nil {
		return derr
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "<details><summary>Full pretty-printed diff</summary>")
	fmt.Fprintln(b)
	fmt.Fprintf(b, "```diff\n%s```\n", ensureTrailingNewline(diff))
	fmt.Fprintln(b)
	fmt.Fprintln(b, "</details>")
	return nil
}

// CatalogDiff is the structural difference between two parsed price catalogs.
// Model entries are keyed "providerID/modelID".
type CatalogDiff struct {
	ProvidersAdded   []string
	ProvidersRemoved []string
	ModelsAdded      []string
	ModelsRemoved    []string
	ModelsChanged    []string
}

func (d CatalogDiff) empty() bool {
	return len(d.ProvidersAdded) == 0 && len(d.ProvidersRemoved) == 0 &&
		len(d.ModelsAdded) == 0 && len(d.ModelsRemoved) == 0 && len(d.ModelsChanged) == 0
}

// diffCatalog computes the provider- and model-level differences between two
// catalogs. Comparison is semantic: models are compared with reflect.DeepEqual
// on the parsed ModelInfo, so formatting-only changes are ignored.
func diffCatalog(old, newV []genaiprices.Provider) CatalogDiff {
	oldProv := providerIDs(old)
	newProv := providerIDs(newV)
	oldModels := modelMap(old)
	newModels := modelMap(newV)

	var d CatalogDiff
	d.ProvidersAdded = setDiff(newProv, oldProv)
	d.ProvidersRemoved = setDiff(oldProv, newProv)

	for key, m := range newModels {
		if _, ok := oldModels[key]; !ok {
			d.ModelsAdded = append(d.ModelsAdded, key)
		} else if !reflect.DeepEqual(oldModels[key], m) {
			d.ModelsChanged = append(d.ModelsChanged, key)
		}
	}
	for key := range oldModels {
		if _, ok := newModels[key]; !ok {
			d.ModelsRemoved = append(d.ModelsRemoved, key)
		}
	}
	sort.Strings(d.ModelsAdded)
	sort.Strings(d.ModelsRemoved)
	sort.Strings(d.ModelsChanged)
	return d
}

func providerIDs(provs []genaiprices.Provider) map[string]bool {
	ids := make(map[string]bool, len(provs))
	for _, p := range provs {
		ids[p.ID] = true
	}
	return ids
}

func modelMap(provs []genaiprices.Provider) map[string]genaiprices.ModelInfo {
	m := make(map[string]genaiprices.ModelInfo)
	for _, p := range provs {
		for _, model := range p.Models {
			id := model.ID
			if id == "" {
				id = model.Name
			}
			m[p.ID+"/"+id] = model
		}
	}
	return m
}

// setDiff returns the sorted keys present in a but not in b.
func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func emitSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "<details><summary>%s (%d)</summary>\n\n", title, len(items))
	shown := items
	if len(shown) > listLimit {
		shown = shown[:listLimit]
	}
	for _, it := range shown {
		fmt.Fprintf(b, "- `%s`\n", it)
	}
	if len(items) > listLimit {
		fmt.Fprintf(b, "- … %d more\n", len(items)-listLimit)
	}
	fmt.Fprintln(b, "\n</details>")
}

// ---- I/O helpers (shell out to gh / diff) ---------------------------------

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

func indentJSON(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// ---- delivery (stdout or sticky PR comment) -------------------------------

func deliver(report, pr string) error {
	if out := os.Getenv("OUT"); out != "" {
		if err := os.WriteFile(out, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write OUT=%s: %w", out, err)
		}
	}
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
	jqExpr := fmt.Sprintf("[.[] | select(.body | contains(%q)) | .id] | first // empty", marker)
	cmd := exec.Command("gh", "api", "repos/{owner}/{repo}/issues/"+pr+"/comments", "--paginate", "--jq", jqExpr)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("list comments on PR #%s: %v: %s", pr, err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

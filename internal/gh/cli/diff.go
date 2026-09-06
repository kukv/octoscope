package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// PRDiff returns the pull request's diff, one entry per file.
//
// --color never is passed explicitly: gh colours its output when it thinks a
// terminal is watching, and escape sequences in the middle of a line would
// break both the parser and every width calculation downstream.
//
// gh pr diff refuses past 300 files (HTTP 406); the files API has no such
// limit. Rather than matching GitHub's error text for that (its wording is
// not ours to depend on, and .claude/rules/errors.md says not to branch on
// error strings), any failure here is retried through the files API, which
// is cheap since failures are rare. If that also fails, the original error
// is what gets reported: it is the one that describes what the user
// actually asked for.
func (c *Client) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	args := appendRepo(
		[]string{"pr", "diff", strconv.Itoa(number), "--color", "never"},
		c.effectiveRepo(repo),
	)
	out, err := c.run(ctx, c.dir, args...)
	if err != nil {
		files, ferr := c.prFiles(ctx, repo, number)
		if ferr == nil {
			return files, nil
		}
		// err is the one that describes what the user actually asked for and
		// is reported as such (see fail() in internal/tui/app); ferr is
		// joined in rather than discarded so a bug in prFiles itself (a
		// parse failure, say) is not silently swallowed
		// (.claude/rules/errors.md). errors.Is still matches err through the
		// join.
		return nil, errors.Join(err, ferr)
	}
	return parseDiff(out), nil
}

// prFiles is the files-API fallback for PRDiff. --paginate is required: the
// endpoint's default page is 30 files, and the pull request this fallback
// exists for had 418. per_page=100 cuts that down to 5 requests instead of
// 14 (ListAssignees in cli.go does the same for its own listing).
func (c *Client) prFiles(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	out, err := c.run(ctx, c.dir, "api", prFilesPath(c.effectiveRepo(repo), number), "--paginate")
	if err != nil {
		return nil, err
	}
	var entries []prFileJSON
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse pr files: %w", err)
	}
	files := make([]gh.FileDiff, len(entries))
	for i, e := range entries {
		files[i] = e.toDomain()
	}
	return files, nil
}

// prFilesPath builds the REST path for the files-API fallback. gh api
// substitutes {owner}/{repo} from the current directory's repo when no
// repository is named (see ListAssignees); an override is spelled out
// explicitly instead, because gh api takes no --repo flag.
func prFilesPath(repo string, number int) string {
	repoPart := "{owner}/{repo}"
	if repo != "" {
		repoPart = repo
	}
	return fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", repoPart, number)
}

// prFileJSON is one entry of the files API's response.
type prFileJSON struct {
	Filename         string  `json:"filename"`
	PreviousFilename string  `json:"previous_filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Patch            *string `json:"patch"`
}

// toDomain converts one files-API entry. Patch is a pointer because GitHub
// omits the field entirely for a file it declines to send a diff for (too
// large, or binary); that is PatchOmitted, not Binary, since the files API
// gives no way to tell a binary file apart from any other reason GitHub left
// the patch out.
func (e prFileJSON) toDomain() gh.FileDiff {
	f := gh.FileDiff{
		Path:      e.Filename,
		OldPath:   e.PreviousFilename,
		Status:    fileStatusFromAPI(e.Status),
		Additions: e.Additions,
		Deletions: e.Deletions,
	}
	if e.Patch == nil {
		f.PatchOmitted = true
		return f
	}
	f.Hunks = parseBarePatch(*e.Patch)
	return f
}

// fileStatusFromAPI translates the files API's status spelling. GitHub says
// "removed", not "deleted"; anything unrecognised (including "modified")
// falls back to FileModified.
func fileStatusFromAPI(s string) gh.FileStatus {
	switch s {
	case "added":
		return gh.FileAdded
	case "removed":
		return gh.FileDeleted
	case "renamed":
		return gh.FileRenamed
	case "copied":
		return gh.FileCopied
	case "changed":
		return gh.FileChanged
	case "unchanged":
		return gh.FileUnchanged
	default:
		return gh.FileModified
	}
}

// parseBarePatch reads the hunks out of a files-API patch: unified diff
// hunks with no "diff --git" header and no ---/+++ lines. It walks the same
// diffParser used for a full gh pr diff, entering it already "inside" a
// file, so the hunk-header parsing (hunkStarts, with its function-context
// fix) is shared rather than duplicated.
func parseBarePatch(patch string) []gh.Hunk {
	p := &diffParser{file: &gh.FileDiff{}}
	s := bufio.NewScanner(strings.NewReader(patch))
	s.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for s.Scan() {
		p.line(s.Text())
	}
	p.closeHunk()
	return p.file.Hunks
}

// parseDiff reads a unified diff. It never fails: a line it does not
// recognise inside a hunk is dropped, and one outside a hunk is a header we
// have no use for. A diff that half-parses shows a file short; a parser that
// returns an error shows nothing at all, which is worse.
func parseDiff(b []byte) []gh.FileDiff {
	p := &diffParser{}
	s := bufio.NewScanner(bytes.NewReader(b))
	// A single diff line can be far longer than bufio's default 64KiB limit
	// (a minified bundle is one line), and a scanner that gives up mid-file
	// would drop every file after it.
	s.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for s.Scan() {
		p.line(s.Text())
	}
	return p.done()
}

// diffParser holds the walk's position: which file and hunk are open, and
// how far down each side of the file the next line falls.
type diffParser struct {
	files        []gh.FileDiff
	file         *gh.FileDiff
	hunk         *gh.Hunk
	oldNo, newNo int
}

func (p *diffParser) closeHunk() {
	if p.file != nil && p.hunk != nil {
		p.file.Hunks = append(p.file.Hunks, *p.hunk)
	}
	p.hunk = nil
}

func (p *diffParser) closeFile() {
	p.closeHunk()
	if p.file != nil {
		p.files = append(p.files, *p.file)
	}
	p.file = nil
}

func (p *diffParser) done() []gh.FileDiff {
	p.closeFile()
	return p.files
}

func (p *diffParser) line(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.closeFile()
		p.file = &gh.FileDiff{Status: gh.FileModified, Path: pathFromGitHeader(line)}
	case p.file == nil:
		// Anything before the first "diff --git" is not ours.
	case strings.HasPrefix(line, "new file mode"):
		p.file.Status = gh.FileAdded
	case strings.HasPrefix(line, "deleted file mode"):
		p.file.Status = gh.FileDeleted
	case strings.HasPrefix(line, "rename from "):
		p.file.Status = gh.FileRenamed
		p.file.OldPath = strings.TrimPrefix(line, "rename from ")
	case strings.HasPrefix(line, "rename to "):
		p.file.Status = gh.FileRenamed
		p.file.Path = strings.TrimPrefix(line, "rename to ")
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		p.file.Binary = true
	case strings.HasPrefix(line, "@@"):
		p.closeHunk()
		p.oldNo, p.newNo = hunkStarts(line)
		p.hunk = &gh.Hunk{Header: line}
	case p.hunk == nil:
		// --- / +++ / index, and anything else before the first hunk.
	case strings.HasPrefix(line, `\`):
		// "\ No newline at end of file" annotates the line above it; it is
		// not a line of the file.
	case strings.HasPrefix(line, "+"):
		p.add(gh.DiffLine{Kind: gh.LineAdded, NewLine: p.newNo, Text: line[1:]})
		p.newNo++
		p.file.Additions++
	case strings.HasPrefix(line, "-"):
		p.add(gh.DiffLine{Kind: gh.LineRemoved, OldLine: p.oldNo, Text: line[1:]})
		p.oldNo++
		p.file.Deletions++
	default:
		// A context line starts with a space. An empty line in the file can
		// arrive as the empty string when the trailing space was stripped in
		// transit, which is why this is the default rather than a " " case.
		p.add(gh.DiffLine{
			Kind: gh.LineContext, OldLine: p.oldNo, NewLine: p.newNo,
			Text: strings.TrimPrefix(line, " "),
		})
		p.oldNo++
		p.newNo++
	}
}

func (p *diffParser) add(l gh.DiffLine) { p.hunk.Lines = append(p.hunk.Lines, l) }

// pathFromGitHeader reads the new path out of `diff --git a/x b/y`. A path
// containing a space makes the two halves ambiguous, so the b/ half is taken
// from the last " b/" in the line, which is where git puts it.
func pathFromGitHeader(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	i := strings.LastIndex(rest, " b/")
	if i < 0 {
		return strings.TrimPrefix(rest, "a/")
	}
	return rest[i+len(" b/"):]
}

// hunkStarts reads the first line number of each side out of
// `@@ -12,7 +12,9 @@ func Walk(...)`. Only the two fields between the leading
// and trailing `@@` carry the range; git appends the enclosing function's
// name after the second `@@` (its xfuncname heuristic), and a one-line
// function body is exactly the shape that puts a `+` or `-` token there
// too, so fields past the second `@@` must not be scanned for a range.
func hunkStarts(header string) (oldNo, newNo int) {
	fields := strings.Fields(header)
	for _, f := range fields[:min(3, len(fields))] {
		switch {
		case strings.HasPrefix(f, "-"):
			oldNo = firstNumber(f[1:])
		case strings.HasPrefix(f, "+"):
			newNo = firstNumber(f[1:])
		}
	}
	return oldNo, newNo
}

func firstNumber(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0 // a header we cannot read still draws; it just numbers from 0.
	}
	return n
}

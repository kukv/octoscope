package gh

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	// A single diff line can exceed bufio's default 64KiB limit (a minified
	// bundle is one line), and the scanner then stops mid-file.
	scanBufInit = 64 * 1024
	scanBufMax  = 8 * 1024 * 1024

	// Everything after the second @@ in `@@ -12,7 +12,9 @@ func Walk(...)` is
	// git's enclosing-function heuristic, which can itself start with + or -.
	hunkHeaderFields = 3
)

// ParseDiff reads a unified diff. It never fails: a line it does not
// recognise inside a hunk is dropped, and one outside a hunk is a header we
// have no use for. A diff that half-parses shows a file short; a parser that
// returns an error shows nothing at all, which is worse.
func ParseDiff(b []byte) []FileDiff {
	p := &diffParser{}
	s := bufio.NewScanner(bytes.NewReader(b))
	s.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for s.Scan() {
		p.line(s.Text())
	}
	return p.done()
}

// ParseFilesAPI decodes the files API's answer, which is what a diff falls
// back to. Both backends read the same shape: the cli one through gh api, the
// api one through the endpoint itself.
func ParseFilesAPI(out []byte) ([]FileDiff, error) {
	var entries []prFileJSON
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse pr files: %w", err)
	}
	files := make([]FileDiff, len(entries))
	for i, e := range entries {
		files[i] = e.toDomain()
	}
	return files, nil
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
func (e prFileJSON) toDomain() FileDiff {
	f := FileDiff{
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
func fileStatusFromAPI(s string) FileStatus {
	switch s {
	case "added":
		return FileAdded
	case "removed":
		return FileDeleted
	case "renamed":
		return FileRenamed
	case "copied":
		return FileCopied
	case "changed":
		return FileChanged
	case "unchanged":
		return FileUnchanged
	default:
		return FileModified
	}
}

// parseBarePatch reads the hunks out of a files-API patch: unified diff
// hunks with no "diff --git" header and no ---/+++ lines. It walks the same
// diffParser used for a full gh pr diff, entering it already "inside" a
// file, so the hunk-header parsing (hunkStarts, with its function-context
// fix) is shared rather than duplicated.
func parseBarePatch(patch string) []Hunk {
	p := &diffParser{file: &FileDiff{}}
	s := bufio.NewScanner(strings.NewReader(patch))
	s.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for s.Scan() {
		p.line(s.Text())
	}
	p.closeHunk()
	return p.file.Hunks
}

// diffParser holds the walk's position: which file and hunk are open, and
// how far down each side of the file the next line falls.
type diffParser struct {
	files        []FileDiff
	file         *FileDiff
	hunk         *Hunk
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

func (p *diffParser) done() []FileDiff {
	p.closeFile()
	return p.files
}

func (p *diffParser) line(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.closeFile()
		p.file = &FileDiff{Status: FileModified, Path: pathFromGitHeader(line)}
	case p.file == nil:
		// Anything before the first "diff --git" is not ours.
	case strings.HasPrefix(line, "new file mode"):
		p.file.Status = FileAdded
	case strings.HasPrefix(line, "deleted file mode"):
		p.file.Status = FileDeleted
	case strings.HasPrefix(line, "rename from "):
		p.file.Status = FileRenamed
		p.file.OldPath = strings.TrimPrefix(line, "rename from ")
	case strings.HasPrefix(line, "rename to "):
		p.file.Status = FileRenamed
		p.file.Path = strings.TrimPrefix(line, "rename to ")
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		p.file.Binary = true
	case strings.HasPrefix(line, "@@"):
		p.closeHunk()
		p.oldNo, p.newNo = hunkStarts(line)
		p.hunk = &Hunk{Header: line}
	case p.hunk == nil:
		// --- / +++ / index, and anything else before the first hunk.
	case strings.HasPrefix(line, `\`):
		// "\ No newline at end of file" annotates the line above it; it is
		// not a line of the file.
	case strings.HasPrefix(line, "+"):
		p.add(DiffLine{Kind: LineAdded, NewLine: p.newNo, Text: line[1:]})
		p.newNo++
		p.file.Additions++
	case strings.HasPrefix(line, "-"):
		p.add(DiffLine{Kind: LineRemoved, OldLine: p.oldNo, Text: line[1:]})
		p.oldNo++
		p.file.Deletions++
	default:
		// A context line starts with a space. An empty line in the file can
		// arrive as the empty string when the trailing space was stripped in
		// transit, which is why this is the default rather than a " " case.
		p.add(DiffLine{
			Kind: LineContext, OldLine: p.oldNo, NewLine: p.newNo,
			Text: strings.TrimPrefix(line, " "),
		})
		p.oldNo++
		p.newNo++
	}
}

func (p *diffParser) add(l DiffLine) { p.hunk.Lines = append(p.hunk.Lines, l) }

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
	for _, f := range fields[:min(hunkHeaderFields, len(fields))] {
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

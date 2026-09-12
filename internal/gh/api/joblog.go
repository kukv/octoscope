package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/kukv/octoscope/internal/gh"
)

// jobNameLimit is how many UTF-16 code units of a job's name the server keeps
// when it builds the zip. The limit is there to stay under a zip path length
// limit, and it counts the way the server's runtime counts: a character
// outside the basic plane takes two.
const jobNameLimit = 90

// logFileName is what a job is called inside the log archive. The server drops
// the characters it cannot put in a path and cuts the rest short, so a job's
// own name does not find its entry.
func logFileName(jobName string) string {
	name := strings.ReplaceAll(jobName, "/", "")
	name = strings.ReplaceAll(name, ":", "")
	if units := utf16.Encode([]rune(name)); len(units) > jobNameLimit {
		name = string(utf16.Decode(units[:jobNameLimit]))
	}
	return strings.TrimSpace(name)
}

// sanitizeControls replaces the control characters a log line may carry with
// caret notation. A log is output from somewhere else that ends up on this
// terminal, and an escape character in it would be acted on rather than shown.
// Tab, newline, vertical tab and carriage return are left alone: they are how
// the runner lays output out, and so is delete, which gh leaves alone too.
func sanitizeControls(line string) string {
	var b strings.Builder
	for _, r := range line {
		if c, ok := caret(r); ok {
			b.WriteString(c)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// caret is a control character written the way a terminal shows one: ^ and the
// letter it is made from. C1 characters (0x80-0x9f) show as the C0 characters
// they mirror. Anything else is returned untouched, delete (0x7f) included:
// it is a control character Go names as one and gh does not rewrite.
func caret(r rune) (string, bool) {
	switch {
	case r == '\t' || r == '\n' || r == '\v' || r == '\r':
		return "", false
	case r >= 0x00 && r <= 0x1f:
		return "^" + string('@'+r), true
	case r >= 0x80 && r <= 0x9f:
		return "^" + string('@'+r-0x80), true
	}
	return "", false
}

// jobLogLines reads the job's log out of the run's archive. GitHub builds one
// zip per run, so a single job's log is a matter of finding its entries in it.
//
// The order is the one gh reads in: per-step entries when the archive has any,
// the job's whole log when it does not, and the job's own endpoint when the
// archive names neither. An archive that cannot be fetched or read at all is
// an error rather than a fourth case: gh stops there too, and answering with
// the job endpoint would hide a failure behind a log with no step names.
func (c *Client) jobLogLines(ctx context.Context, repo string, j job, failedOnly bool) ([]gh.LogLine, error) {
	archive, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/runs/%d/logs", repo, j.RunID), "")
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("read the run's log archive: %w", err)
	}

	lines, ok, err := stepLines(zr, j, failedOnly)
	if err != nil {
		return nil, err
	}
	if ok {
		return lines, nil
	}
	if f := match(zr, wholeJobPattern(j.Name)); f != nil {
		return readEntry(f, unknownStep)
	}
	return c.wholeJobLog(ctx, repo, j)
}

// unknownStep is what a line is attributed to when only the job's whole log is
// available: the file holds every step's output with no boundary between them.
// It is the words gh puts there, so both backends show the same thing.
const unknownStep = "UNKNOWN STEP"

// stepLines reads one entry per step. ok is false when the archive has no
// per-step entry for this job at all, which is the caller's signal to fall
// back rather than to show an empty log.
func stepLines(zr *zip.Reader, j job, failedOnly bool) (lines []gh.LogLine, ok bool, err error) {
	steps := slices.Clone(j.Steps)
	slices.SortFunc(steps, func(a, b jobStep) int { return a.Number - b.Number })

	dir := logFileName(j.Name)
	for _, s := range steps {
		f := match(zr, stepPattern(dir, s.Number))
		if f == nil {
			continue
		}
		ok = true
		if failedOnly && !failed(s.Conclusion) {
			continue
		}
		entry, entryErr := readEntry(f, s.Name)
		if entryErr != nil {
			return nil, false, entryErr
		}
		lines = append(lines, entry...)
	}
	return lines, ok, nil
}

// wholeJobLog reads the job's own log endpoint, which answers with plain text
// rather than an archive. There are no step boundaries in it.
func (c *Client) wholeJobLog(ctx context.Context, repo string, j job) ([]gh.LogLine, error) {
	body, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, j.ID), "")
	if err != nil {
		return nil, err
	}
	return logLines(string(body), unknownStep), nil
}

func readEntry(f *zip.File, step string) ([]gh.LogLine, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("read log entry %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read log entry %s: %w", f.Name, err)
	}
	return logLines(string(body), step), nil
}

// logLines splits one entry into the lines the screen shows. A blank line in
// the middle of a log is kept: gh prints one for it too, and dropping it here
// would put a shorter log on the screen than the cli backend shows.
//
// An entry with nothing in it is no lines at all, which is why the empty body
// is answered before the split: splitting "" yields one empty line.
func logLines(body, step string) []gh.LogLine {
	if body == "" {
		return nil
	}
	var lines []gh.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		lines = append(lines, gh.NewLogLine(step, sanitizeControls(strings.TrimSuffix(raw, "\r"))))
	}
	return lines
}

// stepPattern is where a step's output sits: under the job's directory, named
// after the step's number and the step's own name. The name after the number
// is the server's, not ours, so only the number is matched.
func stepPattern(dir string, number int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^%s/%d_.*\.txt$`, regexp.QuoteMeta(dir), number))
}

// wholeJobPattern is the top-level entry holding one job's entire log. The
// number in front is an ordinal that says nothing about which job it is, and
// the older Actions service puts a negative job id there instead.
func wholeJobPattern(jobName string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^-?\d+_%s\.txt$`, regexp.QuoteMeta(logFileName(jobName))))
}

func match(zr *zip.Reader, re *regexp.Regexp) *zip.File {
	for _, f := range zr.File {
		if re.MatchString(f.Name) {
			return f
		}
	}
	return nil
}

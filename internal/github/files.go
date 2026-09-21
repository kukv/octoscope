package github

import (
	"encoding/json"
	"fmt"
)

type PRFile struct {
	Filename         string  `json:"filename"`
	PreviousFilename string  `json:"previous_filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Patch            *string `json:"patch"`
}

// ParseFilesAPI decodes the files API's answer, which is what a diff falls
// back to. Both backends read the same shape: the cli one through gh api, the
// api one through the endpoint itself.
func ParseFilesAPI(out []byte) ([]PRFile, error) {
	var files []PRFile
	if err := json.Unmarshal(out, &files); err != nil {
		return nil, fmt.Errorf("parse pr files: %w", err)
	}
	return files, nil
}

// Diff is what a PRDiff call answers. Exactly one of Raw and Files is set:
// Raw is the pull request's unified diff text, the common case; Files is
// what the files-API fallback returns instead, when the diff itself could
// not be fetched (gh pr diff refuses past 300 files; GitHub's REST diff
// media type has an analogous limit).
type Diff struct {
	Raw   []byte
	Files []PRFile
}

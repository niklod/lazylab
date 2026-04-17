package models

import "strings"

type MRDiffFile struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

type MRDiffData struct {
	Files []MRDiffFile `json:"files,omitempty"`
}

// DiffStats counts added/removed lines across a diff. Matches what `git
// diff --numstat` would produce, excluding the `+++`/`---` file-header
// lines that are metadata, not content changes.
type DiffStats struct {
	Added   int
	Removed int
}

// Stats walks every file's unified diff and tallies added/removed lines.
// File header lines (`+++ b/foo`, `--- a/foo`) are skipped; hunk headers
// (`@@ … @@`) and context lines are ignored.
func (d *MRDiffData) Stats() DiffStats {
	var s DiffStats
	if d == nil {
		return s
	}
	for i := range d.Files {
		for _, line := range strings.Split(d.Files[i].Diff, "\n") {
			switch {
			case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
				continue
			case strings.HasPrefix(line, "+"):
				s.Added++
			case strings.HasPrefix(line, "-"):
				s.Removed++
			}
		}
	}

	return s
}

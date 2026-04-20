package models

import (
	"strconv"
	"time"
)

type DiscussionStats struct {
	TotalResolvable int `json:"total_resolvable"`
	Resolved        int `json:"resolved"`
}

// Discussion is a single discussion thread on a merge request. The first
// entry in Notes is the "head" — its Resolvable flag drives whether the
// thread itself is resolvable.
type Discussion struct {
	ID    string `json:"id"`
	Notes []Note `json:"notes"`
}

// Note is a single comment inside a discussion. System notes (bot-written
// lifecycle events — "resolved thread", "assigned @x") carry System = true
// and are filtered out at the UI layer.
type Note struct {
	ID         int           `json:"id"`
	Body       string        `json:"body"`
	Author     User          `json:"author"`
	CreatedAt  time.Time     `json:"created_at"`
	Resolvable bool          `json:"resolvable"`
	Resolved   bool          `json:"resolved"`
	ResolvedBy *User         `json:"resolved_by,omitempty"`
	System     bool          `json:"system"`
	Position   *NotePosition `json:"position,omitempty"`
}

// NotePosition locates a code-linked note. NewLine is populated for lines
// present in the new revision; OldLine for lines removed. Either may be
// zero independently.
type NotePosition struct {
	NewPath string `json:"new_path,omitempty"`
	OldPath string `json:"old_path,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
	OldLine int    `json:"old_line,omitempty"`
}

// Head returns the first note in the discussion, or nil for an empty
// discussion.
func (d *Discussion) Head() *Note {
	if d == nil || len(d.Notes) == 0 {
		return nil
	}

	return &d.Notes[0]
}

// IsResolvable reports whether the discussion is a review thread (its head
// note carries the resolvable flag). General/non-threaded comments return
// false.
func (d *Discussion) IsResolvable() bool {
	head := d.Head()
	if head == nil {
		return false
	}

	return head.Resolvable
}

// IsResolved reports whether every resolvable note in the discussion has
// been resolved. Mirrors the aggregation rule in
// gitlab.listMRDiscussionsRaw.
func (d *Discussion) IsResolved() bool {
	if d == nil {
		return false
	}
	if !d.IsResolvable() {
		return false
	}
	for _, n := range d.Notes {
		if n.Resolvable && !n.Resolved {
			return false
		}
	}

	return true
}

// Replies returns every note after the head.
func (d *Discussion) Replies() []Note {
	if d == nil || len(d.Notes) <= 1 {
		return nil
	}

	return d.Notes[1:]
}

// VisibleNotes returns the notes that should be rendered — i.e. all
// non-system notes. Preserves slice order. Fast path: no system notes → we
// return the underlying slice without copying. Callers MUST NOT mutate the
// result.
func (d *Discussion) VisibleNotes() []Note {
	if d == nil {
		return nil
	}
	for i := range d.Notes {
		if d.Notes[i].System {
			out := make([]Note, 0, len(d.Notes))
			for _, n := range d.Notes {
				if !n.System {
					out = append(out, n)
				}
			}

			return out
		}
	}

	return d.Notes
}

// VisibleNoteCount returns the count of non-system notes without allocating.
// Preferred over len(d.VisibleNotes()) on hot paths — j/k cursor movement
// calls this per keypress.
func (d *Discussion) VisibleNoteCount() int {
	if d == nil {
		return 0
	}
	n := 0
	for i := range d.Notes {
		if !d.Notes[i].System {
			n++
		}
	}

	return n
}

// Resolver returns the user who resolved the thread — the ResolvedBy of the
// last resolved note, or nil when the thread is still open / has never been
// resolved / resolver metadata is missing.
func (d *Discussion) Resolver() *User {
	if !d.IsResolved() {
		return nil
	}
	for i := len(d.Notes) - 1; i >= 0; i-- {
		n := d.Notes[i]
		if n.Resolvable && n.Resolved && n.ResolvedBy != nil {
			return n.ResolvedBy
		}
	}

	return nil
}

// LocationHint builds the "path:line" fragment shown next to a thread
// header when the head note is code-linked. Returns "" for general
// comments. Prefers NewPath/NewLine (the revision under review); falls
// back to OldPath/OldLine when only those are populated.
func (d *Discussion) LocationHint() string {
	head := d.Head()
	if head == nil || head.Position == nil {
		return ""
	}

	return head.Position.Location()
}

// Location renders the note position as "path:line". Returns "" when
// neither new nor old side carries a path.
func (p *NotePosition) Location() string {
	if p == nil {
		return ""
	}
	path, line := p.NewPath, p.NewLine
	if path == "" {
		path, line = p.OldPath, p.OldLine
	}
	if path == "" {
		return ""
	}
	if line <= 0 {
		return path
	}

	return path + ":" + strconv.Itoa(line)
}

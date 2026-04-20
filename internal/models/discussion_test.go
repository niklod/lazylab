package models_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type DiscussionSuite struct {
	suite.Suite
}

func (s *DiscussionSuite) TestDiscussionStats_JSONRoundTrip() {
	d := models.DiscussionStats{TotalResolvable: 5, Resolved: 3}

	data, err := json.Marshal(d)
	s.Require().NoError(err)

	var decoded models.DiscussionStats
	s.Require().NoError(json.Unmarshal(data, &decoded))
	s.Require().Equal(d, decoded)
}

func (s *DiscussionSuite) TestHead_Empty() {
	var d models.Discussion
	s.Require().Nil(d.Head())

	var nilD *models.Discussion
	s.Require().Nil(nilD.Head())
}

func (s *DiscussionSuite) TestHead_FirstNote() {
	d := models.Discussion{Notes: []models.Note{
		{ID: 1, Body: "first"},
		{ID: 2, Body: "second"},
	}}

	head := d.Head()

	s.Require().NotNil(head)
	s.Require().Equal(1, head.ID)
	s.Require().Equal("first", head.Body)
}

func (s *DiscussionSuite) TestIsResolvable_GeneralComment() {
	d := models.Discussion{Notes: []models.Note{{Resolvable: false}}}
	s.Require().False(d.IsResolvable())
}

func (s *DiscussionSuite) TestIsResolvable_ReviewThread() {
	d := models.Discussion{Notes: []models.Note{{Resolvable: true}}}
	s.Require().True(d.IsResolvable())
}

func (s *DiscussionSuite) TestIsResolved_RequiresAllResolvableResolved() {
	fullyResolved := models.Discussion{Notes: []models.Note{
		{Resolvable: true, Resolved: true},
		{Resolvable: true, Resolved: true},
	}}
	partial := models.Discussion{Notes: []models.Note{
		{Resolvable: true, Resolved: true},
		{Resolvable: true, Resolved: false},
	}}
	unresolvable := models.Discussion{Notes: []models.Note{{Resolvable: false}}}

	s.Require().True(fullyResolved.IsResolved())
	s.Require().False(partial.IsResolved())
	s.Require().False(unresolvable.IsResolved())
}

func (s *DiscussionSuite) TestReplies() {
	single := models.Discussion{Notes: []models.Note{{ID: 1}}}
	multi := models.Discussion{Notes: []models.Note{{ID: 1}, {ID: 2}, {ID: 3}}}

	s.Require().Empty(single.Replies())
	s.Require().Len(multi.Replies(), 2)
	s.Require().Equal(2, multi.Replies()[0].ID)
}

func (s *DiscussionSuite) TestVisibleNotes_SkipsSystem() {
	d := models.Discussion{Notes: []models.Note{
		{ID: 1, Body: "user"},
		{ID: 2, Body: "bot", System: true},
		{ID: 3, Body: "user-2"},
	}}

	visible := d.VisibleNotes()

	s.Require().Len(visible, 2)
	s.Require().Equal(1, visible[0].ID)
	s.Require().Equal(3, visible[1].ID)
}

func (s *DiscussionSuite) TestResolver_LastResolvedNoteUser() {
	resolver := &models.User{Username: "alice"}
	d := models.Discussion{Notes: []models.Note{
		{Resolvable: true, Resolved: true, ResolvedBy: &models.User{Username: "bob"}},
		{Resolvable: true, Resolved: true, ResolvedBy: resolver},
	}}

	got := d.Resolver()

	s.Require().NotNil(got)
	s.Require().Equal("alice", got.Username)
}

func (s *DiscussionSuite) TestResolver_UnresolvedThread() {
	d := models.Discussion{Notes: []models.Note{{Resolvable: true, Resolved: false}}}
	s.Require().Nil(d.Resolver())
}

func (s *DiscussionSuite) TestLocationHint_NewPath() {
	d := models.Discussion{Notes: []models.Note{{Position: &models.NotePosition{NewPath: "foo.go", NewLine: 42}}}}
	s.Require().Equal("foo.go:42", d.LocationHint())
}

func (s *DiscussionSuite) TestLocationHint_FallbackOldPath() {
	d := models.Discussion{Notes: []models.Note{{Position: &models.NotePosition{OldPath: "legacy.go", OldLine: 7}}}}
	s.Require().Equal("legacy.go:7", d.LocationHint())
}

func (s *DiscussionSuite) TestLocationHint_NoPosition() {
	d := models.Discussion{Notes: []models.Note{{Body: "general"}}}
	s.Require().Empty(d.LocationHint())
}

func (s *DiscussionSuite) TestLocationHint_PathWithoutLine() {
	d := models.Discussion{Notes: []models.Note{{Position: &models.NotePosition{NewPath: "foo.go"}}}}
	s.Require().Equal("foo.go", d.LocationHint())
}

func (s *DiscussionSuite) TestLocation_NilReceiver() {
	var p *models.NotePosition
	s.Require().Empty(p.Location())
}

func (s *DiscussionSuite) TestIsResolvable_SystemHeadRealReply() {
	d := models.Discussion{Notes: []models.Note{
		{System: true, Resolvable: false},
		{System: false, Resolvable: true},
	}}

	s.Require().False(d.IsResolvable(),
		"system head drives IsResolvable — a real reply behind it does not promote the thread")
	s.Require().Len(d.VisibleNotes(), 1, "system note filtered from VisibleNotes")
	s.Require().Equal(1, d.VisibleNoteCount(), "count helper matches VisibleNotes length")
}

func (s *DiscussionSuite) TestVisibleNotes_FastPathSharesBackingArray() {
	d := models.Discussion{Notes: []models.Note{
		{ID: 1, Body: "a"},
		{ID: 2, Body: "b"},
	}}

	got := d.VisibleNotes()

	s.Require().Len(got, 2)
	if len(got) > 0 && len(d.Notes) > 0 {
		s.Require().Same(&d.Notes[0], &got[0],
			"no system notes → VisibleNotes returns the underlying slice without copying")
	}
}

func TestDiscussionSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DiscussionSuite))
}

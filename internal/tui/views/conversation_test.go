package views

import (
	"strings"
	"testing"
	"time"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
)

type ConversationViewSuite struct {
	suite.Suite
	view *ConversationView
}

func (s *ConversationViewSuite) SetupTest() {
	s.view = NewConversation()
}

func (s *ConversationViewSuite) TestInitialState_IsLoading() {
	rows := s.view.SnapshotRows()

	s.Require().Empty(rows)
}

func (s *ConversationViewSuite) TestSetDiscussions_BucketsByResolvableAndResolved() {
	discs := []*models.Discussion{
		reviewThread("unresolved-1", false),
		reviewThread("resolved-1", true),
		generalComment("general-1"),
	}

	s.view.SetDiscussions(discs)

	s.Require().Equal(2, s.view.ThreadCount())
	s.Require().Equal(1, s.view.UnresolvedCount())
}

func (s *ConversationViewSuite) TestSetDiscussions_SkipsEmptyAndAllSystem() {
	discs := []*models.Discussion{
		{ID: "empty"},
		{ID: "bots-only", Notes: []models.Note{{System: true}}},
		reviewThread("real", false),
	}

	s.view.SetDiscussions(discs)

	s.Require().Equal(1, s.view.ThreadCount())
}

func (s *ConversationViewSuite) TestSetDiscussions_ResetsCursorToFirstSelectable() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("a", false),
		reviewThread("b", false),
	})

	thread, note := s.view.Cursor()

	s.Require().Equal(0, thread)
	s.Require().Equal(0, note)
}

func (s *ConversationViewSuite) TestMoveThreadCursor_AdvancesByAnchorRows() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("a", false),
		reviewThread("b", false),
		reviewThread("c", false),
	})

	moved := s.view.MoveThreadCursor(1)

	thread, _ := s.view.Cursor()
	s.Require().True(moved)
	s.Require().Equal(1, thread)
}

func (s *ConversationViewSuite) TestMoveThreadCursor_ClampsAtEnd() {
	s.view.SetDiscussions([]*models.Discussion{reviewThread("a", false)})

	s.view.MoveThreadCursor(10)
	thread, _ := s.view.Cursor()

	s.Require().Equal(0, thread, "single-thread → no movement possible")
}

func (s *ConversationViewSuite) TestMoveNoteCursor_WalksNotesWithinThread() {
	d := &models.Discussion{
		ID: "multi",
		Notes: []models.Note{
			{ID: 1, Body: "head", Resolvable: true},
			{ID: 2, Body: "reply1", Resolvable: true},
			{ID: 3, Body: "reply2", Resolvable: true},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})

	moved := s.view.MoveNoteCursor(2)

	_, note := s.view.Cursor()
	s.Require().True(moved)
	s.Require().Equal(2, note)
}

func (s *ConversationViewSuite) TestMoveNoteCursor_WalksToLastNote() {
	d := &models.Discussion{
		ID: "multi",
		Notes: []models.Note{
			{ID: 1, Body: "head", Resolvable: true, Author: models.User{Username: "a"}},
			{ID: 2, Body: "r1", Resolvable: true, Author: models.User{Username: "b"}},
			{ID: 3, Body: "r2", Resolvable: true, Author: models.User{Username: "c"}},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})

	for i := 0; i < 10; i++ {
		s.view.MoveNoteCursor(1)
	}

	_, note := s.view.Cursor()
	s.Require().Equal(3, note,
		"cursor range is 0..N (inclusive); last note must be reachable")
}

func (s *ConversationViewSuite) TestMoveNoteCursor_NoopOnGeneralCommentAnchor() {
	s.view.SetDiscussions([]*models.Discussion{generalComment("g1")})

	moved := s.view.MoveNoteCursor(1)

	_, note := s.view.Cursor()
	s.Require().False(moved)
	s.Require().Equal(0, note)
}

func (s *ConversationViewSuite) TestMoveNoteCursor_NoopForSingleNoteThread() {
	s.view.SetDiscussions([]*models.Discussion{reviewThread("a", false)})

	s.Require().True(s.view.MoveNoteCursor(1),
		"single-note thread still allows stepping from header (0) to the note (1)")
	s.Require().False(s.view.MoveNoteCursor(1),
		"further stepping clamps at the last note")
}

func (s *ConversationViewSuite) TestToggleExpandAll_PreservesCursorOnUnresolvedThread() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("open-1", false),
		reviewThread("open-2", false),
		reviewThread("done-1", true),
		reviewThread("done-2", true),
	})
	s.view.MoveThreadCursor(1)
	beforeThread, _ := s.view.Cursor()
	s.Require().Equal(1, beforeThread, "cursor on second unresolved thread")

	s.view.ToggleExpandAllResolved()

	afterThread, afterNote := s.view.Cursor()
	s.Require().Equal(1, afterThread,
		"cursor stays on the unresolved thread identity across mass-toggle")
	s.Require().Equal(0, afterNote)
}

func (s *ConversationViewSuite) TestToggleExpandUnderCursor_PreservesCursorOnResolvedThread() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("open-1", false),
		reviewThread("done-1", true),
		reviewThread("done-2", true),
	})
	s.view.MoveThreadCursor(2)
	beforeThread, _ := s.view.Cursor()
	s.Require().Equal(2, beforeThread, "cursor on second resolved thread (collapsed)")

	s.Require().True(s.view.ToggleExpandResolvedUnderCursor())

	afterThread, afterNote := s.view.Cursor()
	s.Require().Equal(2, afterThread,
		"cursor tracks the resolved thread through its own expand")
	s.Require().Equal(0, afterNote)
}

func (s *ConversationViewSuite) TestToggleExpandUnderCursor_OnlyExpandsTargetedThread() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("done-1", true),
		reviewThread("done-2", true),
	})
	s.view.MoveThreadCursor(0)

	s.Require().True(s.view.ToggleExpandResolvedUnderCursor())

	s.Require().True(s.view.IsResolvedThreadExpanded(0),
		"cursor's thread is expanded")
	s.Require().False(s.view.IsResolvedThreadExpanded(1),
		"untargeted resolved thread stays collapsed")
	s.Require().False(s.view.ExpandAllResolved())
}

func (s *ConversationViewSuite) TestToggleExpandUnderCursor_NoopOnUnresolvedThread() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("open", false),
		reviewThread("done", true),
	})
	s.view.MoveThreadCursor(0)

	s.Require().False(s.view.ToggleExpandResolvedUnderCursor(),
		"e on an unresolved thread is a no-op")
	s.Require().False(s.view.IsResolvedThreadExpanded(0))
}

func (s *ConversationViewSuite) TestToggleExpandAll_ExpandsAndCollapsesEveryResolvedThread() {
	s.view.SetDiscussions([]*models.Discussion{
		reviewThread("open", false),
		reviewThread("done-1", true),
		reviewThread("done-2", true),
	})
	before := rowKindsOf(s.view)
	s.Require().Contains(before, rowKindResolvedCollapsed)

	s.view.ToggleExpandAllResolved()

	after := rowKindsOf(s.view)
	s.Require().True(s.view.ExpandAllResolved())
	s.Require().Contains(after, rowKindResolvedHeader, "every resolved thread expanded")

	s.view.ToggleExpandAllResolved()

	s.Require().False(s.view.ExpandAllResolved())
	collapsedAgain := rowKindsOf(s.view)
	s.Require().Contains(collapsedAgain, rowKindResolvedCollapsed,
		"second mass-toggle collapses everything")
}

func (s *ConversationViewSuite) TestRender_ShowsLoadingHintBeforeSetDiscussions() {
	pane := newBufferPane(s.T())

	s.view.Render(pane)

	s.Require().Contains(pane.Buffer(), conversationLoadingHint)
}

func (s *ConversationViewSuite) TestRender_ShowsEmptyHintWhenNoDiscussions() {
	s.view.SetDiscussions(nil)
	pane := newBufferPane(s.T())

	s.view.Render(pane)

	s.Require().Contains(pane.Buffer(), conversationEmptyHint)
}

func (s *ConversationViewSuite) TestRender_ShowsErrorMessage() {
	s.view.ShowError("boom")
	pane := newBufferPane(s.T())

	s.view.Render(pane)

	s.Require().Contains(pane.Buffer(), "boom")
}

func (s *ConversationViewSuite) TestRender_DrawsThreadHeaderAndNoteBody() {
	frozenNow := time.Now()
	conversationNow = func() time.Time { return frozenNow }
	s.T().Cleanup(func() { conversationNow = time.Now })

	d := &models.Discussion{
		ID: "abc12345xxx",
		Notes: []models.Note{
			{
				ID:         1,
				Body:       "Is the bounds check needed here?",
				Author:     models.User{Username: "jay"},
				CreatedAt:  frozenNow.Add(-2 * 24 * time.Hour),
				Resolvable: true,
			},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().Contains(buf, "Thread · abc12345")
	s.Require().Contains(buf, "unresolved")
	s.Require().Contains(buf, "@jay")
	s.Require().Contains(buf, "Is the bounds check needed here?")
	s.Require().Contains(buf, "2 days ago")
}

func (s *ConversationViewSuite) TestRender_CollapsedResolvedLineIncludesResolverAndReplyCount() {
	resolver := &models.User{Username: "mira.k"}
	d := &models.Discussion{
		ID: "r1",
		Notes: []models.Note{
			{ID: 1, Body: "open", Resolvable: true, Resolved: true, ResolvedBy: resolver, Author: models.User{Username: "a"}},
			{ID: 2, Body: "reply", Resolvable: true, Resolved: true, ResolvedBy: resolver, Author: models.User{Username: "b"}},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().Contains(buf, "resolved by @mira.k")
	s.Require().Contains(buf, "1 replies")
}

func (s *ConversationViewSuite) TestRender_GeneralCommentsSectionBelowDivider() {
	d := &models.Discussion{
		ID: "g1",
		Notes: []models.Note{
			{ID: 1, Body: "Let's ship it.", Author: models.User{Username: "devon"}, Resolvable: false},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().Contains(buf, conversationGeneralHeader)
	s.Require().Contains(buf, "@devon")
	s.Require().Contains(buf, "Let's ship it.")
}

func (s *ConversationViewSuite) TestRender_KeybindStripAppendedToPane() {
	s.view.SetDiscussions([]*models.Discussion{reviewThread("a", false)})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().Contains(buf, "expand all")
	s.Require().Contains(buf, "thread")
}

func (s *ConversationViewSuite) TestSetDiscussions_SkipsSystemNotesInDisplay() {
	d := &models.Discussion{
		ID: "s1",
		Notes: []models.Note{
			{ID: 1, Body: "real", Author: models.User{Username: "u"}, Resolvable: true},
			{ID: 2, Body: "bot said something", System: true, Resolvable: true},
			{ID: 3, Body: "real 2", Author: models.User{Username: "u"}, Resolvable: true},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().NotContains(buf, "bot said something")
	s.Require().Contains(buf, "real")
	s.Require().Contains(buf, "real 2")
}

func (s *ConversationViewSuite) TestChoose_SpineGlyphsByPosition() {
	s.Require().Equal(convSpineSingle, chooseSpine(1, true))
	s.Require().Equal(convSpineMiddle, chooseSpine(3, false))
	s.Require().Equal(convSpineEnd, chooseSpine(3, true))
}

func (s *ConversationViewSuite) TestChromeMeta_CountsOnlyResolvableThreads() {
	discs := []*models.Discussion{
		reviewThread("a", false),
		reviewThread("b", true),
		generalComment("g"),
	}

	meta := conversationChromeMeta(discs)

	s.Require().Equal("Conversation · 2 threads (1 unresolved)", meta)
}

func (s *ConversationViewSuite) TestChromeMeta_NilInput() {
	s.Require().Equal("Conversation · 0 threads (0 unresolved)",
		conversationChromeMeta(nil))
}

func (s *ConversationViewSuite) TestChromeMeta_GeneralOnly_ReportsZeroThreads() {
	discs := []*models.Discussion{generalComment("g1"), generalComment("g2")}

	s.Require().Equal("Conversation · 0 threads (0 unresolved)",
		conversationChromeMeta(discs),
		"general comments are excluded from thread counts")
}

func (s *ConversationViewSuite) TestWrapBodyWithIndent_ContinuationKeepsIndent() {
	indent := "  │     "
	body := "lorem ipsum dolor sit amet consectetur adipiscing elit"

	got := wrapBodyWithIndent(body, indent, 20)

	lines := strings.Split(got, "\n")
	s.Require().Greater(len(lines), 1, "long body must wrap")
	for i, line := range lines {
		s.Require().True(strings.HasPrefix(line, indent),
			"wrap line %d missing indent prefix: %q", i, line)
	}
}

func (s *ConversationViewSuite) TestWrapBodyWithIndent_PreservesBlankLogicalLines() {
	got := wrapBodyWithIndent("first\n\nsecond", " > ", 40)

	lines := strings.Split(got, "\n")
	s.Require().Len(lines, 3)
	s.Require().Equal(" > first", lines[0])
	s.Require().Equal(" > ", lines[1])
	s.Require().Equal(" > second", lines[2])
}

func (s *ConversationViewSuite) TestSoftWrapLine_HardSplitsOverflowingWord() {
	got := softWrapLine("abcdefghij klm", 4)

	s.Require().Equal([]string{"abcd", "efgh", "ij", "klm"}, got)
}

func (s *ConversationViewSuite) TestSanitizeInline_StripsControlChars() {
	input := "hi\x1b[31mEVIL\x1b[0m\rworld\x00\x7fend"

	s.Require().Equal("hiEVILworldend", sanitizeInline(input),
		"ESC sequences, CR, NUL, DEL stripped; printable bytes preserved")
}

func (s *ConversationViewSuite) TestRender_BodyWithANSIEscapeIsSanitized() {
	d := &models.Discussion{
		ID: "evil",
		Notes: []models.Note{
			{
				ID:         1,
				Body:       "click \x1b[31mHERE\x1b[0m",
				Author:     models.User{Username: "mallory\x1b[42m"},
				Resolvable: true,
				CreatedAt:  time.Now().Add(-time.Hour),
			},
		},
	}
	s.view.SetDiscussions([]*models.Discussion{d})
	pane := newBufferPane(s.T())

	s.view.Render(pane)
	buf := pane.Buffer()

	s.Require().Contains(buf, "click HERE", "body escape sequences stripped")
	s.Require().NotContains(buf, "\x1b[31m", "author-embedded SGR never reaches the pane")
	s.Require().NotContains(buf, "\x1b[42m", "author-embedded SGR never reaches the pane")
}

func TestConversationViewSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ConversationViewSuite))
}

func reviewThread(id string, resolved bool) *models.Discussion {
	return &models.Discussion{
		ID: id,
		Notes: []models.Note{
			{
				ID:         1,
				Body:       "note for " + id,
				Author:     models.User{Username: "alice"},
				CreatedAt:  time.Now().Add(-time.Hour),
				Resolvable: true,
				Resolved:   resolved,
				ResolvedBy: resolverFor(resolved),
			},
		},
	}
}

func resolverFor(resolved bool) *models.User {
	if !resolved {
		return nil
	}

	return &models.User{Username: "alice"}
}

func generalComment(id string) *models.Discussion {
	return &models.Discussion{
		ID: id,
		Notes: []models.Note{
			{
				ID:         1,
				Body:       "general " + id,
				Author:     models.User{Username: "carol"},
				CreatedAt:  time.Now().Add(-time.Hour),
				Resolvable: false,
			},
		},
	}
}

func rowKindsOf(v *ConversationView) []rowKind {
	rows := v.SnapshotRows()
	out := make([]rowKind, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.kind)
	}

	return out
}

func newBufferPane(t interface {
	Fatalf(string, ...any)
	Cleanup(func())
}) *gocui.View {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	if err != nil {
		t.Fatalf("NewGui: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	pv, err := g.SetView("conv", 0, 0, 100, 30, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		t.Fatalf("SetView: %v", err)
	}

	return pv
}

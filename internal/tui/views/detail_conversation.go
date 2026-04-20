package views

import (
	"context"
	"strconv"

	"github.com/jesseduffield/gocui"

	"github.com/niklod/lazylab/internal/models"
)

// Conversation returns the Conversation-tab widget. Package-internal use for
// rendering via the layout callback.
func (d *DetailView) Conversation() *ConversationView { return d.conversation }

// beginConversationLoad bumps the load sequence and flips the widget into
// the loading-hint state. Returns the new sequence the caller must carry
// through to applyConversation.
func (d *DetailView) beginConversationLoad() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.conversationSeq++
	if d.conversation != nil {
		d.conversation.ShowLoading()
	}

	return d.conversationSeq
}

// fetchConversationAsync kicks off a background discussions fetch. Safe to
// call when the app/gui are nil (test setup skips the fetch).
func (d *DetailView) fetchConversationAsync(ctx context.Context, project *models.Project, mr *models.MergeRequest) {
	if d.app == nil || d.app.GitLab == nil || d.g == nil || project == nil || mr == nil {
		return
	}
	seq := d.beginConversationLoad()
	projectID := project.ID
	iid := mr.IID
	go func() {
		data, err := d.app.GitLab.ListMRDiscussions(ctx, projectID, iid)
		d.g.Update(func(_ *gocui.Gui) error {
			d.applyConversation(seq, data, err)

			return nil
		})
	}()
}

// applyConversation lands a fetch result into the widget, dropping stale
// results. Mirrors applyDiff / applyPipeline.
func (d *DetailView) applyConversation(seq uint64, data []*models.Discussion, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if seq != d.conversationSeq {
		return
	}
	if err != nil {
		if d.conversation != nil {
			d.conversation.ShowError("fetch mr discussions: " + err.Error())
		}

		return
	}
	d.conversationData = data
	if d.conversation != nil {
		d.conversation.SetDiscussions(data)
		d.conversation.SetChrome(d.conversationChromeTitleLocked(), conversationChromeMeta(data))
	}
}

func (d *DetailView) conversationChromeTitleLocked() string {
	if d.mr == nil {
		return ""
	}

	return "Detail · !" + strconv.Itoa(d.mr.IID)
}

// conversationChromeMeta renders "Conversation · N threads (M unresolved)".
// General comments are excluded from both counts per the wireframe meta.
// Result is cached on the widget in applyConversation — the render tick
// reads the cached string rather than recomputing per frame.
func conversationChromeMeta(discs []*models.Discussion) string {
	threads, unresolved := 0, 0
	for _, d := range discs {
		if d == nil || !d.IsResolvable() {
			continue
		}
		threads++
		if !d.IsResolved() {
			unresolved++
		}
	}

	return "Conversation · " + strconv.Itoa(threads) + " threads (" + strconv.Itoa(unresolved) + " unresolved)"
}

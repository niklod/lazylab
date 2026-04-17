package models

type MRState string

const (
	MRStateOpened MRState = "opened"
	MRStateClosed MRState = "closed"
	MRStateMerged MRState = "merged"
)

func (s MRState) IsOpen() bool   { return s == MRStateOpened }
func (s MRState) IsClosed() bool { return s == MRStateClosed }
func (s MRState) IsMerged() bool { return s == MRStateMerged }

func (s MRState) IsValid() bool {
	switch s {
	case MRStateOpened, MRStateClosed, MRStateMerged:
		return true
	}

	return false
}

type MRStateFilter string

const (
	MRStateFilterAll    MRStateFilter = "all"
	MRStateFilterOpened MRStateFilter = "opened"
	MRStateFilterClosed MRStateFilter = "closed"
	MRStateFilterMerged MRStateFilter = "merged"
)

func (f MRStateFilter) IsValid() bool {
	switch f {
	case MRStateFilterAll, MRStateFilterOpened, MRStateFilterClosed, MRStateFilterMerged:
		return true
	}

	return false
}

type MROwnerFilter string

const (
	MROwnerFilterAll      MROwnerFilter = "all"
	MROwnerFilterMine     MROwnerFilter = "mine"
	MROwnerFilterReviewer MROwnerFilter = "reviewer"
)

func (f MROwnerFilter) IsValid() bool {
	switch f {
	case MROwnerFilterAll, MROwnerFilterMine, MROwnerFilterReviewer:
		return true
	}

	return false
}

type PipelineStatus string

const (
	PipelineStatusCreated            PipelineStatus = "created"
	PipelineStatusWaitingForResource PipelineStatus = "waiting_for_resource"
	PipelineStatusPreparing          PipelineStatus = "preparing"
	PipelineStatusPending            PipelineStatus = "pending"
	PipelineStatusRunning            PipelineStatus = "running"
	PipelineStatusSuccess            PipelineStatus = "success"
	PipelineStatusFailed             PipelineStatus = "failed"
	PipelineStatusCanceled           PipelineStatus = "canceled"
	PipelineStatusSkipped            PipelineStatus = "skipped"
	PipelineStatusManual             PipelineStatus = "manual"
	PipelineStatusScheduled          PipelineStatus = "scheduled"
)

func (s PipelineStatus) IsValid() bool {
	switch s {
	case PipelineStatusCreated, PipelineStatusWaitingForResource, PipelineStatusPreparing,
		PipelineStatusPending, PipelineStatusRunning, PipelineStatusSuccess,
		PipelineStatusFailed, PipelineStatusCanceled, PipelineStatusSkipped,
		PipelineStatusManual, PipelineStatusScheduled:
		return true
	}

	return false
}

func (s PipelineStatus) IsTerminal() bool {
	switch s {
	case PipelineStatusSuccess, PipelineStatusFailed, PipelineStatusCanceled, PipelineStatusSkipped:
		return true
	}

	return false
}

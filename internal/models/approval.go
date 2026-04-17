package models

type ApprovalStatus struct {
	Approved          bool   `json:"approved"`
	ApprovalsRequired int    `json:"approvals_required"`
	ApprovalsLeft     int    `json:"approvals_left"`
	ApprovedBy        []User `json:"approved_by,omitempty"`
}

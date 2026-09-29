// Package authz is the authorization model: what a caller may do, to what, in
// which event, and why.
//
// # Why actions rather than permissions
//
// A permission is a property of a role. An action is a property of the thing
// being done, and that inversion is what makes per-event and per-object control
// possible at all.
//
// With permissions, "may this organizer edit this event?" has no answer that is
// not "yes, for every event", because the role has no event in it. Every event
// is then fenced off by convention, by a slug check, or by nothing. That is the
// state this package was written to replace: the old model granted
// ManageEvent globally and left per-event scoping to each handler's discretion.
//
// With actions, the same question is answered by a target that carries the event
// and the object, and by grants that can be scoped to both.
//
// # The vocabulary is closed
//
// Actions are a declared set, not free strings. A typo in an action name is a
// compile error rather than a route that silently permits everyone, which is the
// failure mode a stringly-typed permission model has by construction.
//
// # One resolver
//
// Every authorization in the portal goes through Resolve. Routes declare the
// action they need and the target they are acting on; the resolver returns a
// Decision carrying the reason, so "why can this judge see this?" is answerable
// and the answer is auditable.
package authz

import "fmt"

// Action is a resource-and-verb pair, written "resource.verb".
type Action string

// The closed action vocabulary. Adding an action here is the only way to add
// one; nothing constructs an Action from a request.
const (
	// Event lifecycle.
	ActionEventCreate        Action = "event.create"
	ActionEventRead          Action = "event.read"
	ActionEventUpdate        Action = "event.update"
	ActionEventDelete        Action = "event.delete"
	ActionEventPublishResult Action = "event.publish_results"
	ActionEventTransferOwner Action = "event.transfer_ownership"

	// Event configuration: tracks, prizes, questions, milestones, hosts.
	ActionEventConfigure Action = "event.configure"

	// Staff, roster and panel.
	ActionEventStaffManage Action = "event.staff_manage"
	ActionPanelManage      Action = "panel.manage"
	ActionJudgeConflict    Action = "panel.declare_conflict"
	// A comparison is a first-class answer to "which of these two", recorded
	// rather than inferred from rubric scores.
	ActionCompareWrite Action = "comparison.write"

	// Teams and membership.
	ActionTeamCreate     Action = "team.create"
	ActionTeamUpdate     Action = "team.update"
	ActionTeamDelete     Action = "team.delete"
	ActionTeamInvite     Action = "team.invite"
	ActionTeamManageUser Action = "team.manage_member"

	// Submissions.
	ActionSubmissionCreate        Action = "submission.create"
	ActionSubmissionRead          Action = "submission.read"
	ActionSubmissionUpdateOwn     Action = "submission.update_own"
	ActionSubmissionUpdateAny     Action = "submission.update_any"
	ActionSubmissionSubmit        Action = "submission.submit"
	ActionSubmissionWithdraw      Action = "submission.withdraw"
	ActionSubmissionLock          Action = "submission.lock"
	ActionSubmissionDisqualify    Action = "submission.disqualify"
	ActionSubmissionDelete        Action = "submission.delete"
	ActionSubmissionSetEligiblity Action = "submission.set_eligibility"

	// Duplicate review.
	ActionDuplicateScan    Action = "duplicate.scan"
	ActionDuplicateResolve Action = "duplicate.resolve"

	// Judging.
	ActionReviewWriteOwn   Action = "review.write_own"
	ActionReviewSubmit     Action = "review.submit"
	ActionReviewReadOwn    Action = "review.read_own"
	ActionReviewReadPeer   Action = "review.read_peer"
	ActionReviewCompare    Action = "review.compare"
	ActionReviewReopen     Action = "review.reopen"
	ActionAssignmentRead   Action = "assignment.read"
	ActionAssignmentCreate Action = "assignment.create"
	ActionAssignmentRevoke Action = "assignment.revoke"
	ActionAssignmentBulk   Action = "assignment.bulk_create"
	ActionRubricCreate     Action = "rubric.create"
	ActionRubricUpdate     Action = "rubric.update"
	ActionRubricPublish    Action = "rubric.publish"
	ActionRubricArchive    Action = "rubric.archive"
	ActionResultsRead      Action = "results.read"
	ActionResultsPublish   Action = "results.publish"
	ActionProgressRead     Action = "progress.read"

	// Public directory reads. These are deliberately distinct from
	// ActionAccountReadAny, which is staff authority over an account. A visitor
	// may read a public profile; that is not the same as being able to read
	// anyone's account record.
	ActionDirectoryRead  Action = "directory.read"
	ActionCommentRead    Action = "comment.read"
	ActionEventDirectory Action = "event.directory"

	// Community.
	ActionVoteCast           Action = "vote.cast"
	ActionVoteCampaignManage Action = "vote.campaign_manage"
	ActionCommentCreate      Action = "comment.create"
	ActionCommentDeleteOwn   Action = "comment.delete_own"
	ActionCommentModerate    Action = "comment.moderate"
	ActionCommentReport      Action = "comment.report"
	ActionCommentReadHidden  Action = "comment.read_hidden"

	// Accounts and platform administration.
	ActionAccountReadSelf      Action = "account.read_self"
	ActionAccountUpdateSelf    Action = "account.update_self"
	ActionAccountExportSelf    Action = "account.export_self"
	ActionAccountDeleteSelf    Action = "account.delete_self"
	ActionAccountReadAny       Action = "account.read_any"
	ActionAccountSuspend       Action = "account.suspend"
	ActionAccountSetRole       Action = "account.set_role"
	ActionAccountRevokeSession Action = "account.revoke_session"
	ActionSessionReadOwn       Action = "session.read_own"
	ActionSessionRevokeOwn     Action = "session.revoke_own"
	ActionSessionReadAny       Action = "session.read_any"
	ActionNotificationReadOwn  Action = "notification.read_own"

	// Platform operations.
	ActionPlatformManage    Action = "platform.manage"
	ActionIntegrationManage Action = "integration.manage"
	ActionMailSend          Action = "mail.send"
	ActionMailRead          Action = "mail.read"
	ActionMailFlush         Action = "mail.flush"

	// Data movement and audit.
	ActionExportCSV   Action = "export.csv"
	ActionExportFull  Action = "export.full"
	ActionImportApply Action = "import.apply"
	ActionAuditRead   Action = "audit.read"
	ActionAuditExport Action = "audit.export"
	ActionAuditVerify Action = "audit.verify"
	ActionGrantManage Action = "grant.manage"
	ActionImpersonate Action = "platform.impersonate"
)

// allActions is the closed set, used for validation, the published matrix, and
// tests that must not silently lose an action.
var allActions = []Action{
	ActionEventCreate, ActionEventRead, ActionEventUpdate, ActionEventDelete,
	ActionEventPublishResult, ActionEventTransferOwner, ActionEventConfigure,
	ActionEventStaffManage, ActionPanelManage, ActionJudgeConflict,
	ActionTeamCreate, ActionTeamUpdate, ActionTeamDelete, ActionTeamInvite, ActionTeamManageUser,
	ActionSubmissionCreate, ActionSubmissionRead, ActionSubmissionUpdateOwn, ActionSubmissionUpdateAny,
	ActionSubmissionSubmit, ActionSubmissionWithdraw, ActionSubmissionLock, ActionSubmissionDisqualify,
	ActionSubmissionDelete, ActionSubmissionSetEligiblity,
	ActionDuplicateScan, ActionDuplicateResolve,
	ActionReviewWriteOwn, ActionReviewSubmit, ActionReviewReadOwn, ActionReviewReadPeer,
	// A comparison is a first-class answer to "which of these two", recorded
	// rather than inferred from rubric scores.
	ActionCompareWrite, ActionReviewCompare, ActionReviewReopen, ActionAssignmentRead, ActionAssignmentCreate,
	ActionAssignmentRevoke, ActionAssignmentBulk, ActionRubricCreate, ActionRubricUpdate,
	ActionRubricPublish, ActionRubricArchive, ActionResultsRead, ActionResultsPublish, ActionProgressRead,
	ActionDirectoryRead, ActionCommentRead, ActionEventDirectory,
	ActionVoteCast, ActionVoteCampaignManage, ActionCommentCreate, ActionCommentDeleteOwn,
	ActionCommentModerate, ActionCommentReport, ActionCommentReadHidden,
	ActionAccountReadSelf, ActionAccountUpdateSelf, ActionAccountExportSelf, ActionAccountDeleteSelf,
	ActionAccountReadAny, ActionAccountSuspend, ActionAccountSetRole, ActionAccountRevokeSession,
	ActionSessionReadOwn, ActionSessionRevokeOwn, ActionSessionReadAny, ActionNotificationReadOwn,
	ActionPlatformManage, ActionIntegrationManage, ActionMailSend, ActionMailRead, ActionMailFlush,
	ActionExportCSV, ActionExportFull, ActionImportApply, ActionAuditRead, ActionAuditExport,
	ActionAuditVerify, ActionGrantManage, ActionImpersonate,
}

var actionSet = func() map[Action]struct{} {
	set := make(map[Action]struct{}, len(allActions))
	for _, action := range allActions {
		set[action] = struct{}{}
	}
	return set
}()

// All returns every declared action, sorted. The HTTP permission matrix and the
// documentation are generated from it, so they cannot drift from the code.
func All() []Action {
	out := make([]Action, len(allActions))
	copy(out, allActions)
	return out
}

// Valid reports whether an action is in the closed set.
func (a Action) Valid() bool {
	_, ok := actionSet[a]
	return ok
}

// Resource is the part of an action before the dot.
func (a Action) Resource() string {
	for i := 0; i < len(a); i++ {
		if a[i] == '.' {
			return string(a[:i])
		}
	}
	return string(a)
}

// String makes Action printable in audit records and log lines.
func (a Action) String() string { return string(a) }

// ParseAction validates a caller-supplied action string.
func ParseAction(value string) (Action, error) {
	action := Action(value)
	if !action.Valid() {
		return "", fmt.Errorf("unknown action %q", value)
	}
	return action, nil
}

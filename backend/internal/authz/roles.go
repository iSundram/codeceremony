package authz

import (
	"sort"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

// RoleDefaults is what each global role may do everywhere, absent any event
// role, grant or deny.
//
// This is the coarse layer. It exists so that a participant can edit their own
// team without anyone configuring anything, and so that an organizer is useful
// the moment they are made one. What it deliberately does not do is grant
// anything event-scoped to a role that another human being on that event might
// also hold: see EventRoleGrants for the part that is fenced per event.
var RoleDefaults = map[domain.Role][]Action{
	domain.RoleVisitor: {
		ActionEventRead,
		ActionEventDirectory,
		ActionDirectoryRead,
		ActionCommentRead,
		ActionSubmissionRead,
		ActionCommentCreate,
		ActionCommentReport,
	},
	domain.RoleParticipant: {
		ActionEventRead,
		ActionSubmissionRead,
		ActionSubmissionCreate,
		ActionSubmissionUpdateOwn,
		ActionSubmissionSubmit,
		ActionSubmissionWithdraw,
		ActionCommentCreate,
		ActionCommentDeleteOwn,
		ActionCommentReport,
		ActionVoteCast,
		ActionTeamCreate,
		ActionTeamUpdate,
		ActionTeamInvite,
		ActionTeamManageUser,
		ActionAccountReadSelf,
		ActionAccountUpdateSelf,
		ActionDirectoryRead,
		ActionCommentRead,
		ActionEventDirectory,
		ActionAccountExportSelf,
		ActionAccountDeleteSelf,
		ActionSessionReadOwn,
		ActionSessionRevokeOwn,
		ActionNotificationReadOwn,
	},
	domain.RoleJudge: {
		ActionEventRead,
		ActionSubmissionRead,
		ActionReviewWriteOwn, ActionCompareWrite,
		ActionReviewSubmit,
		ActionReviewReadOwn,
		ActionReviewCompare,
		ActionAssignmentRead,
		ActionJudgeConflict,
		ActionCommentCreate,
		ActionCommentDeleteOwn,
		ActionCommentReport,
		ActionVoteCast,
		ActionAccountReadSelf,
		ActionAccountUpdateSelf,
		ActionDirectoryRead,
		ActionCommentRead,
		ActionEventDirectory,
		ActionAccountExportSelf,
		ActionAccountDeleteSelf,
		ActionSessionReadOwn,
		ActionSessionRevokeOwn,
		ActionNotificationReadOwn,
	},
	domain.RoleOrganizer: {
		// An organizer runs their own events, so creating one is theirs to do.
		// The previous coarse model granted this through the same permission as
		// editing, and dropping it would have been a silent capability
		// regression for every existing organizer.
		ActionEventCreate,
		ActionEventRead,
		ActionEventUpdate,
		ActionEventConfigure,
		ActionEventPublishResult,
		ActionSubmissionRead,
		ActionSubmissionUpdateAny,
		ActionSubmissionLock,
		ActionSubmissionDisqualify,
		ActionSubmissionSetEligiblity,
		ActionDuplicateScan,
		ActionDuplicateResolve,
		ActionReviewReadPeer,
		ActionAssignmentRead,
		ActionAssignmentCreate,
		ActionAssignmentRevoke,
		ActionAssignmentBulk,
		ActionRubricCreate,
		ActionRubricUpdate,
		ActionRubricPublish,
		ActionRubricArchive,
		ActionResultsRead,
		ActionResultsPublish,
		ActionProgressRead,
		ActionTeamUpdate,
		ActionTeamDelete,
		ActionPanelManage,
		ActionVoteCampaignManage,
		ActionCommentModerate,
		ActionCommentReadHidden,
		ActionExportCSV,
		ActionExportFull,
		ActionImportApply,
		ActionAuditRead,
		// Exporting and verifying the chain, and managing grants, are
		// event-owner jobs. Each is fenced by the event on the target, so an
		// organizer reaches them for their own events and nothing else. The
		// export in particular is what makes the chain useful: an organizer
		// who cannot produce the log cannot hand it to anyone to check.
		ActionAuditExport,
		ActionAuditVerify,
		ActionGrantManage,
		ActionIntegrationManage,
		ActionMailSend,
		ActionMailRead,
		ActionCommentCreate,
		ActionCommentDeleteOwn,
		ActionCommentReport,
		ActionVoteCast,
		ActionAccountReadSelf,
		ActionAccountUpdateSelf,
		ActionDirectoryRead,
		ActionCommentRead,
		ActionEventDirectory,
		ActionAccountExportSelf,
		ActionAccountDeleteSelf,
		ActionSessionReadOwn,
		ActionSessionRevokeOwn,
		ActionSessionReadAny,
		ActionAccountRevokeSession,
		ActionNotificationReadOwn,
	},
	domain.RoleAdmin: {
		ActionEventRead,
		ActionEventUpdate,
		ActionEventCreate,
		ActionEventConfigure,
		ActionEventDelete,
		ActionEventPublishResult,
		ActionEventTransferOwner,
		ActionEventStaffManage,
		ActionPanelManage,
		ActionJudgeConflict,
		ActionSubmissionRead,
		ActionSubmissionUpdateAny,
		ActionSubmissionLock,
		ActionSubmissionDisqualify,
		ActionSubmissionDelete,
		ActionSubmissionSetEligiblity,
		ActionDuplicateScan,
		ActionDuplicateResolve,
		ActionReviewReadPeer,
		ActionReviewReopen,
		ActionAssignmentRead,
		ActionAssignmentCreate,
		ActionAssignmentRevoke,
		ActionAssignmentBulk,
		ActionRubricCreate,
		ActionRubricUpdate,
		ActionRubricPublish,
		ActionRubricArchive,
		ActionResultsRead,
		ActionResultsPublish,
		ActionProgressRead,
		ActionTeamCreate,
		ActionTeamUpdate,
		ActionTeamDelete,
		ActionTeamInvite,
		ActionTeamManageUser,
		ActionVoteCampaignManage,
		ActionCommentModerate,
		ActionCommentReadHidden,
		ActionAccountReadAny,
		ActionAccountSuspend,
		ActionAccountSetRole,
		ActionAccountRevokeSession,
		ActionSessionReadAny,
		ActionPlatformManage,
		ActionIntegrationManage,
		ActionMailSend,
		ActionMailRead,
		ActionMailFlush,
		ActionExportCSV,
		ActionExportFull,
		ActionImportApply,
		ActionAuditRead,
		ActionAuditExport,
		ActionAuditVerify,
		ActionGrantManage,
		ActionImpersonate,
		ActionCommentCreate,
		ActionCommentDeleteOwn,
		ActionCommentReport,
		ActionVoteCast,
		ActionAccountReadSelf,
		ActionAccountUpdateSelf,
		ActionDirectoryRead,
		ActionCommentRead,
		ActionEventDirectory,
		ActionAccountExportSelf,
		ActionAccountDeleteSelf,
		ActionSessionReadOwn,
		ActionSessionRevokeOwn,
		ActionNotificationReadOwn,
	},
}

// EventRoleGrants is what each per-event role may do within that one event.
//
// This is the layer that makes two organizers of different events two separate
// people rather than one global role with extra steps. An organizer with no row
// in this table for an event gets the coarse global default, which is what keeps
// a fresh event usable; a user who *is* on the event's staff is additionally
// governed by their EventRole, and the two intersect.
//
// A per-event role can only narrow. It never widens: a viewer on an event cannot
// do something the global role could not, and an owner cannot act on a platform
// they are not an admin of.
var EventRoleGrants = map[domain.EventRole][]Action{
	domain.EventRoleOwner: {
		ActionEventRead, ActionEventUpdate, ActionEventConfigure, ActionEventDelete,
		ActionEventPublishResult, ActionEventTransferOwner, ActionEventStaffManage,
		ActionPanelManage, ActionJudgeConflict,
		ActionSubmissionRead, ActionSubmissionUpdateAny, ActionSubmissionLock,
		ActionSubmissionDisqualify, ActionSubmissionDelete, ActionSubmissionSetEligiblity,
		ActionDuplicateScan, ActionDuplicateResolve,
		ActionReviewReadPeer, ActionReviewReopen,
		ActionAssignmentRead, ActionAssignmentCreate, ActionAssignmentRevoke, ActionAssignmentBulk,
		ActionRubricCreate, ActionRubricUpdate, ActionRubricPublish, ActionRubricArchive,
		ActionResultsRead, ActionResultsPublish, ActionProgressRead,
		ActionTeamCreate, ActionTeamUpdate, ActionTeamDelete, ActionTeamInvite, ActionTeamManageUser,
		ActionVoteCampaignManage,
		ActionCommentModerate, ActionCommentReadHidden,
		ActionExportCSV, ActionExportFull, ActionImportApply,
		ActionAuditRead, ActionAuditExport, ActionAuditVerify, ActionGrantManage,
		ActionIntegrationManage, ActionMailSend, ActionMailRead,
		ActionAccountReadAny, ActionAccountRevokeSession, ActionSessionReadAny,
	},
	domain.EventRoleCoOrganizer: {
		ActionEventRead, ActionEventUpdate, ActionEventConfigure, ActionEventPublishResult,
		ActionPanelManage, ActionJudgeConflict,
		ActionSubmissionRead, ActionSubmissionUpdateAny, ActionSubmissionLock,
		ActionSubmissionDisqualify, ActionSubmissionSetEligiblity,
		ActionDuplicateScan, ActionDuplicateResolve,
		ActionReviewReadPeer,
		ActionAssignmentRead, ActionAssignmentCreate, ActionAssignmentRevoke, ActionAssignmentBulk,
		ActionRubricCreate, ActionRubricUpdate, ActionRubricPublish, ActionRubricArchive,
		ActionResultsRead, ActionResultsPublish, ActionProgressRead,
		ActionTeamUpdate, ActionTeamInvite, ActionTeamManageUser,
		ActionVoteCampaignManage,
		ActionCommentModerate, ActionCommentReadHidden,
		ActionExportCSV, ActionIntegrationManage, ActionMailSend, ActionMailRead,
		ActionAuditRead,
	},
	domain.EventRoleJudgeLiaison: {
		ActionEventRead, ActionSubmissionRead, ActionPanelManage, ActionJudgeConflict,
		ActionAssignmentRead, ActionAssignmentCreate, ActionAssignmentRevoke, ActionAssignmentBulk,
		ActionReviewReadPeer, ActionResultsRead, ActionProgressRead,
		ActionRubricReadOnly, ActionCommentModerate,
	},
	domain.EventRoleModerator: {
		ActionEventRead, ActionCommentModerate, ActionCommentReadHidden,
		ActionCommentDeleteOwn, ActionCommentCreate, ActionSubmissionRead,
		ActionVoteCampaignManage, ActionDuplicateScan,
	},
	domain.EventRoleViewer: {
		ActionEventRead, ActionSubmissionRead, ActionResultsRead, ActionProgressRead,
	},
}

// ActionRubricReadOnly is not a grantable action; it exists so the judge liaison
// row above can express "may see the rubric, may not change it" without granting
// the update actions. It is declared here rather than in action.go because it is
// never required by a route.
const ActionRubricReadOnly Action = "rubric.read"

// roleDefaultSet and eventRoleSet are membership sets built once, for lookup.
var (
	roleDefaultSet = func() map[domain.Role]map[Action]struct{} {
		out := make(map[domain.Role]map[Action]struct{}, len(RoleDefaults))
		for role, actions := range RoleDefaults {
			set := make(map[Action]struct{}, len(actions))
			for _, action := range actions {
				set[action] = struct{}{}
			}
			out[role] = set
		}
		return out
	}()

	eventRoleSet = func() map[domain.EventRole]map[Action]struct{} {
		out := make(map[domain.EventRole]map[Action]struct{}, len(EventRoleGrants))
		for role, actions := range EventRoleGrants {
			set := make(map[Action]struct{}, len(actions))
			for _, action := range actions {
				set[action] = struct{}{}
			}
			out[role] = set
		}
		return out
	}()
)

// RoleAllows reports whether a global role permits an action, ignoring event
// scope. The resolver consults this first; nothing else may widen it.
func RoleAllows(role domain.Role, action Action) bool {
	actions, ok := roleDefaultSet[role]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

// EventRoleAllows reports whether a per-event role permits an action within that
// event. It can only narrow the global role, so the resolver intersects rather
// than unions.
func EventRoleAllows(role domain.EventRole, action Action) bool {
	actions, ok := eventRoleSet[role]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

// ActionsForRole returns a role's global actions, sorted, for the published
// matrix.
func ActionsForRole(role domain.Role) []Action {
	actions := append([]Action(nil), RoleDefaults[role]...)
	sort.Slice(actions, func(i, j int) bool { return actions[i] < actions[j] })
	return actions
}

// ActionsForEventRole returns an event role's actions, sorted.
func ActionsForEventRole(role domain.EventRole) []Action {
	actions := append([]Action(nil), EventRoleGrants[role]...)
	sort.Slice(actions, func(i, j int) bool { return actions[i] < actions[j] })
	return actions
}

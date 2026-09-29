# CodeCeremony — Account, Access, Team, and Notification Specification
<p align="center">
  <img src="docs/assets/codeceremony-logo.svg" alt="CodeCeremony" width="320" />
</p>

> **Partly superseded.** This is the account and roles design, kept as a record.
> For the enforcement that actually exists, read
> [`ARCHITECTURE.md`](ARCHITECTURE.md) section 5 and
> [`THREAT-MODEL.md`](THREAT-MODEL.md).

> **Status:** Comprehensive product and security decisions.
>
> **Scope:** First-party account management, identity, sessions, roles, permissions, teams, notifications, email, and auditability.
>
> **Read first:** `rules.md`, `FEATURES.md`, `structure.md`, and `design.md`.
>
> **Hard boundary:** CodeCeremony uses its own local identity system. There is no Google, OIDC, OAuth, or other external identity provider. SMTP, if used, is only an optional mail transport for notifications and is never required for login, local startup, or acceptance checks.

## 1. Non-negotiable account rules

1. CodeCeremony accounts are first-party local accounts. No external identity provider is part of the product.
2. Local accounts work without SMTP or any hosted service.
3. SMTP is optional. If it is not configured, notifications remain available in-app and are queued for later delivery.
4. No secret, SMTP password, or integration credential is committed.
5. Authorization is enforced in the backend/API, never only in the UI.
6. Every privileged action is checked against account type, role, event scope, team scope, ownership, resource state, and deadline.
7. Default policy is deny. A missing permission, role, scope, or state check fails closed.
8. Every role change, promotion, demotion, suspension, deletion, session revocation, and permission override creates an audit event.
9. A user must not be able to promote themselves, grant themselves organizer/admin access, or bypass team/event scope.
10. Account deletion is a controlled lifecycle operation, not an unreviewed database delete.
11. Private judge scores, private notes, unpublished results, and security secrets never enter notification or email payloads.
12. No new account type, permission, notification event, or action may be added during implementation without updating this document.

## 2. Account types

### 2.1 Visitor

A visitor is an unauthenticated public actor, not a stored user account.

A visitor may:

- view a public event page;
- view published gallery projects;
- view published results;
- view public certificates and verification records;
- use an explicitly enabled public voting link;
- accept a public comment or voting token if the event policy allows it.

A visitor may not:

- access private submissions;
- access drafts;
- access reviews or scores;
- access organizer or admin data;
- see unpublished results;
- bypass rate limits or anti-abuse controls.

### 2.2 Participant

A participant is an authenticated user participating in one or more events.

A participant may:

- maintain a profile;
- create or join teams;
- accept, decline, or revoke team invitations according to policy;
- view their own teams and submissions;
- create and edit their own team's submission drafts;
- submit before the deadline;
- vote and comment when enabled;
- manage their own sessions and notification preferences;
- view published results and their own participation records.

A participant may not:

- read another team's private drafts;
- read judge scores;
- read unpublished aggregates;
- assign judges;
- change event policy;
- promote themselves;
- delete a team they do not own;
- access organizer exports or platform audit data.

### 2.3 Team member

Team membership is event-scoped and separate from the global account role.

A team member may:

- view the team's private workspace;
- view the team's submission draft if the event policy allows it;
- comment or vote when permitted;
- receive team activity notifications;
- leave the team when the event policy allows it.

A team member may not:

- submit for the team unless they are the captain or have been granted submission permission;
- promote or demote another member;
- delete the team;
- view judge-only material;
- alter the team's ownership without an allowed transfer.

### 2.4 Team leader

A team leader is a delegated team-scoped capability, not a new global account role.

A team leader may:

- edit team metadata;
- invite and revoke members according to event policy;
- approve a submission draft;
- submit for the team if the captain has granted that permission;
- request a captain transfer;
- view team activity and notifications.

A team leader may not:

- grant themselves captain;
- remove or demote the captain;
- change the event's permissions;
- view other teams;
- bypass submission deadlines.

### 2.5 Team captain / owner

The captain is the team owner for the event.

A captain may:

- promote a member to team leader;
- demote a team leader to member;
- remove a member;
- transfer captaincy;
- submit and edit the team's submission;
- delete or archive the team when event policy allows it;
- view the team's complete activity history.

A captain may not:

- promote themselves to a global role;
- grant permissions they do not hold;
- delete an event or another team;
- override a closed deadline;
- erase audit history.

If a team has no captain, the event enters an explicit ` captain_required` state. Organizers can restore a captain or archive the team; they cannot silently assign ownership.

### 2.6 Judge

A judge is an authenticated user with event- and track-scoped judging permissions.

A judge may:

- view assigned projects;
- view the rubric version assigned to them;
- save and submit their own reviews;
- declare a conflict;
- view their own progress;
- view their own review history;
- receive assignment and deadline notifications;
- request removal from a conflicting assignment.

A judge may not:

- view another judge's review or score;
- view an unassigned track;
- view aggregate results before permitted publication;
- change assignments;
- change the rubric;
- view another judge's private notes;
- export organizer-wide data.

Judge scope is checked on every project, review, export, and API response.

### 2.7 Event moderator

Moderator is an event-scoped delegated capability, not a global role by default.

A moderator may:

- review and hide comments;
- review abuse reports;
- invalidate suspicious votes;
- lock or unlock a submission within policy;
- view operational audit data for the event.

A moderator may not:

- change rubric or scoring maths;
- publish results;
- manage platform accounts;
- view judge scores unless separately granted organizer permission;
- access other events.

### 2.8 Organizer

An organizer is scoped to one or more events.

An organizer may:

- create and configure events;
- manage tracks, prizes, deadlines, custom questions, and rubrics;
- invite and assign judges;
- manage teams and eligibility;
- view progress, reviews, normalized results, and audit data;
- configure voting, comments, and moderation;
- publish results;
- issue certificates and participation records;
- import and export event data;
- suspend event-scoped accounts;
- configure event notification policies.

An organizer may not:

- manage platform-wide roles without admin permission;
- read another organizer's event without an explicit grant;
- silently edit a submitted review;
- delete audit records;
- bypass backend permission checks.

### 2.9 Platform admin

An admin is a global, highly privileged account type.

An admin may:

- manage platform users and account status;
- assign global roles;
- create and manage event organizers;
- manage templates and platform configuration;
- view cross-event operational data;
- manage API tokens and webhook integrations;
- revoke sessions;
- merge or anonymize accounts;
- view and export the platform audit trail;
- suspend or recover an organizer;
- run maintenance and migration operations.

Admin actions are always audited and should require a recent authenticated session. High-risk actions may require step-up authentication when the deployment enables it.

### 2.10 API token / service account

An API token is not a human account and has no password or browser session.

An API token has:

- owner;
- explicit scopes;
- event restrictions;
- expiry;
- last-used timestamp;
- revocation state;
- audit history.

A service account is a non-human identity used by local automation, imports, or webhooks. It must be clearly labeled and cannot silently inherit human permissions.

### 2.11 System identity

System identities are generated by migrations and seed processes. They are not assignable to humans and cannot log in through the normal account UI.

Examples:

- fixture seeder;
- migration runner;
- retention worker;
- local development accounts.

## 3. Account states

Account state is separate from role. A suspended judge is still a judge by type but has no active permissions.

| State | Meaning |
|---|---|
| `pending` | Created but not activated or email not verified. |
| `active` | Can authenticate subject to role and policy. |
| `suspended` | Temporarily blocked by an authorized actor. |
| `deactivated` | User-initiated or policy-based logout state; data retained. |
| `locked` | Authentication temporarily blocked after abuse or credential attacks. |
| `deletion_pending` | Awaiting retention/anonymization workflow. |
| `deleted` | Identity anonymized or removed according to retention policy. |
| `merged` | Identity consolidated into another account; old record retained as a tombstone. |

State transitions require an actor, timestamp, reason, and audit event.

## 4. Account profile and settings

A user profile may contain:

- display name;
- avatar reference;
- short biography;
- organization or affiliation;
- country/timezone display preference;
- locale;
- accessibility preferences;
- public profile visibility;
- notification preferences;
- created and last-active timestamps.

Private fields:

- password hash;
- session tokens;
- integration secrets;
- recovery data;
- private judge notes;
- private moderation notes.

A profile change never changes a user's event role, team role, or judge scope automatically.

## 5. Local authentication

Local authentication is the only identity system. It must work without external identity providers.

Required capabilities:

- account creation;
- email normalization;
- password policy;
- adaptive password hashing;
- login;
- logout;
- password change;
- session creation;
- session expiry;
- session revocation;
- account suspension and recovery;
- local development seed identities.

Passwords are never logged, emailed, stored in plaintext, or included in exports.

A password change revokes all other sessions by default. An administrator password reset creates a one-time recovery action and never reveals the previous password.

## 6. First-party identity boundary

CodeCeremony owns its identity system. There is no Google, OIDC, OAuth, social login, external identity provider, or provider account linking.

The only account credential is the local CodeCeremony account:

- email or username;
- password hash;
- server-side session;
- account state;
- role and scoped permissions;
- team membership;
- notification preferences;
- audit history.

The My Account surface is a first-party product area. It is not a mirror of a third-party account console and must not display third-party provider buttons.

### 6.1 My Account sections

The account center is divided into explicit sections:

- profile and identity details;
- email and username;
- password and security;
- active sessions and devices;
- teams and team roles;
- judge assignments and track scopes;
- event participation history;
- notification preferences;
- email delivery status;
- data export;
- account deletion and retention;
- security and activity history.

### 6.2 Account identifier rules

- Email or username changes require re-authentication.
- Email changes require confirmation through the configured local mail transport when available.
- A user cannot change their own global role.
- A user cannot change their own account identifier to impersonate another user.
- Display names are not unique and never grant identity.
- User IDs are stable and are used in audit records.

### 6.3 No external identity state

There are no external identity tokens, identity-provider subjects, identity-provider secrets, or identity-link records in the product. The database must not contain an external identity-link table.

### 6.4 Offline operation

The account system must start and work with no external identity provider. Mail transport is optional and never required for account management:

- local login works;
- sessions work;
- in-app notifications work;
- account data remains locally controlled;
- the acceptance checker is unaffected;
- no network call is required for account management.

## 7. Session management

### 7.1 Session model

A session is a server-side record with:

- session ID;
- user ID;
- hashed token or session secret;
- created time;
- last-seen time;
- absolute expiry;
- idle expiry;
- revocation time;
- revocation reason;
- IP address or privacy-safe network metadata;
- user-agent metadata;
- device label;
- authentication method (`local`, `system`, `api`).

Raw session tokens are never stored in plaintext in the database.

### 7.2 Session types

- browser session;
- API session;
- administrative session;
- organization-invitation session;
- email-recovery session;
- service/API token session.

Administrative sessions may have shorter expiry and stricter step-up requirements.

### 7.3 Session controls

Users can:

- view active sessions;
- view device label and last activity;
- revoke one session;
- revoke all other sessions;
- revoke all sessions;
- sign out everywhere;
- change password and invalidate other sessions;
- enable or disable local account security features when policy allows.

Organizers can revoke sessions for accounts in their event scope. Admins can revoke sessions globally with an audit reason.

### 7.4 Expiry and rotation

- Absolute expiry is mandatory.
- Idle expiry is recommended.
- Tokens rotate after login, password change, privilege elevation, and account recovery.
- Expired or revoked tokens are rejected even if their signature is valid.
- Reusing a rotated or revoked token creates a security audit event.
- Session cookies use HttpOnly, Secure in production, SameSite, and a narrow path.

### 7.5 Session security events

Record:

- login success and failure;
- logout;
- token rotation;
- session revocation;
- all-sessions revocation;
- suspicious reuse;
- account suspension;
- email or username change;
- administrative impersonation, if ever enabled.

## 8. Team roles and lifecycle

### 8.1 Team roles

| Role | Scope | Capabilities |
|---|---|---|
| `member` | Team | View permitted team content, comment, vote, receive team activity. |
| `leader` | Team | Manage team metadata, invitations, member workflow, and approved submission actions. |
| `captain` | Team | Full team control, promotion/demotion, captain transfer, submission ownership, delete/archive request. |

Team roles are event-scoped and stored in membership records. They do not change the global account role.

### 8.2 Promotion

A member can be promoted to leader when:

- the actor is captain or has explicit organizer permission;
- the team is not archived;
- the target is an active member;
- the event permits delegated leadership;
- the action includes a reason and audit event.

A captain can promote at most the configured number of leaders. Self-promotion is impossible.

### 8.3 Demotion

A leader can be demoted to member when:

- the actor is captain or an authorized organizer/admin;
- the target is not the captain;
- the action includes a reason;
- pending actions assigned to that leader are reassigned or recorded;
- the target receives a notification.

The captain cannot be demoted by a team leader.

### 8.4 Captain transfer

Captain transfer requires:

- captain or authorized organizer/admin actor;
- active target member;
- explicit confirmation;
- atomic transfer;
- audit event;
- notification to both users;
- no orphan team.

### 8.5 Member removal and leaving

- A captain or organizer may remove a member according to policy.
- A member may leave unless the team would violate a minimum-member requirement.
- Removal does not erase contribution history.
- Removed members lose access to private team content immediately.
- A removed member may rejoin only through a new invitation.
- The removed user receives a notification and can appeal through an organizer.

### 8.6 Team deletion

Team deletion is a soft, audited operation.

Default behavior:

1. captain or authorized organizer requests deletion;
2. system checks event state, submission state, and retention policy;
3. affected members receive a warning notification;
4. the team is marked `deletion_pending`;
5. the grace period runs;
6. the team is archived or anonymized;
7. submission and audit history is retained according to policy;
8. all members are notified.

Hard deletion is an admin-only maintenance action and requires an explicit reason and a separate confirmation.

An active team cannot be deleted by a team leader or ordinary member.

## 9. Permission model

### 9.1 Permission evaluation

Every protected action is evaluated in a fixed order by one resolver, and the
order is the design rather than an implementation detail:

```text
account state          → suspended or pending-deletion stops here
target constraints     → is this action even applicable to this kind of object
explicit deny          → a recorded deny wins over everything below it
explicit grant         → a recorded allow
event role             → scoped to this event only
global role            → the platform-wide bundle
ownership              → captain, author, assignee
```

A role match alone never grants access, and a role cannot take back what a deny
removed. The order means an incident response is one API call: recording a deny
takes effect on the next request, with no role change and no wait.

Every decision carries a **reason** naming the rule that produced it, and that
reason is what lands in the audit trail — so a refusal is answerable rather than
merely deniable.

### 9.2 Permission catalog

#### Account and session permissions

- `account.view_self`
- `account.update_self`
- `account.change_password`
- `account.view_sessions`
- `account.revoke_own_session`
- `account.revoke_other_sessions`
- `account.link_identity`
- `account.unlink_identity`
- `account.request_deletion`
- `account.export_self`
- `account.delete_self` when policy allows

#### Team permissions

- `team.create`
- `team.view`
- `team.update_metadata`
- `team.invite`
- `team.revoke_invite`
- `team.join`
- `team.leave`
- `team.remove_member`
- `team.promote_leader`
- `team.demote_leader`
- `team.transfer_captaincy`
- `team.submit_project`
- `team.delete`
- `team.archive`

#### Event and submission permissions

- `event.create`
- `event.view`
- `event.update`
- `event.archive`
- `event.configure_tracks`
- `event.configure_prizes`
- `event.configure_deadlines`
- `event.configure_rubric`
- `submission.create`
- `submission.view_private`
- `submission.edit`
- `submission.submit`
- `submission.withdraw`
- `submission.lock`
- `submission.disqualify`
- `submission.export`

#### Judge permissions

- `judge.view_assignment`
- `judge.declare_conflict`
- `judge.save_review`
- `judge.submit_review`
- `judge.view_own_review`
- `judge.request_assignment_change`
- `judge.view_participation_record`

#### Result and public permissions

- `result.view_published`
- `result.view_aggregate_early`
- `result.publish`
- `result.unpublish`
- `result.export`
- `vote.cast`
- `vote.moderate`
- `comment.create`
- `comment.moderate`

#### Administration permissions

- `admin.view_users`
- `admin.search_users`
- `admin.update_user_state`
- `admin.assign_global_role`
- `admin.assign_event_role`
- `admin.revoke_user_sessions`
- `admin.merge_accounts`
- `admin.export_audit`
- `admin.manage_tokens`
- `admin.manage_templates`
- `admin.manage_platform_settings`
- `admin.run_maintenance`

### 9.3 Role inheritance

- A more privileged role does not automatically receive every lower-level permission in every scope.
- Event organizers are scoped to assigned events.
- Team leaders are scoped to one team.
- Judges are scoped to assigned tracks/projects.
- Admin is the only global role, and its actions remain audited.
- A user may hold different capabilities in different events.

### 9.4 Protected actions

The following always require explicit authorization and audit:

- role assignment/removal;
- team captain transfer;
- team deletion;
- account suspension/deletion;
- session revocation of another user;
- email or username change;
- judge assignment or reassignment;
- score or review alteration;
- result publication;
- bulk import;
- API token creation;
- SMTP credential changes;
- impersonation, if ever introduced.

## 10. Admin account management

Admin views support:

- search by ID, email, display name, state, or role;
- filter by event participation and team;
- view account state and last activity;
- view active sessions;
- view account state history;
- view notification delivery state;
- view audit history;
- suspend, restore, lock, deactivate, or delete according to policy;
- merge duplicate accounts;
- revoke sessions;
- assign or remove roles;
- export account data;
- view security events.

Every admin action requires:

- actor identity;
- target identity;
- scope;
- reason;
- timestamp;
- before/after summary;
- request/correlation ID.

Admins cannot edit or delete audit rows through normal application operations.

## 11. Notifications

### 11.1 Delivery channels

- in-app notification inbox;
- email through optional SMTP;
- webhook delivery for external systems;
- digest view for grouped activity.

In-app delivery is always available locally. Email failure never removes or hides the in-app record.

### 11.2 Notification event catalog

#### Account and security

- account created;
- account activated;
- password changed;
- password recovery requested;
- login from a new device;
- suspicious login detected;
- session revoked;
- all sessions revoked;
- email or username changed;
- account suspended;
- account restored;
- account deletion requested;
- account merged.

#### Team

- team created;
- team invitation sent;
- team invitation accepted;
- team invitation declined/revoked/expired;
- member joined;
- member left;
- member removed;
- member promoted to leader;
- leader demoted to member;
- captaincy transferred;
- team deletion requested;
- team archived;
- team submission created/updated/submitted.

#### Event

- event created;
- event configuration changed;
- deadline changed;
- registrations opened/closed;
- submissions opened/closed;
- judging opened/closed;
- voting opened/closed;
- results published/unpublished;
- event archived;
- announcement published.

#### Judging

- judge invited;
- judge assignment created;
- assignment changed/revoked;
- conflict declared;
- review assigned;
- review reminder;
- review saved;
- review submitted;
- review deadline approaching;
- judge participation record issued.

#### Public participation

- comment created;
- comment approved/hidden/removed;
- vote cast;
- vote invalidated;
- abuse report received;
- moderation action taken.

#### Administration and operations

- import started/completed/failed;
- export completed/failed;
- webhook delivery failed;
- SMTP delivery failed;
- API token created/revoked;
- audit export completed.

### 11.3 Notification preferences

Users can choose:

- in-app on/off per category;
- email on/off per category;
- immediate email versus daily/weekly digest;
- quiet hours and timezone;
- muted events;
- one-time security notices that cannot be disabled.

Security, account suspension, deletion, payment, and critical policy notices are mandatory and cannot be disabled.

### 11.4 Notification safety

- Payloads contain only the minimum data needed.
- Judge scores, private notes, unpublished results, tokens, and secrets are excluded.
- Email links contain short-lived, single-purpose, revocable action links.
- Links do not expose raw session tokens.
- Every notification has an idempotency key.
- Retries do not create duplicate emails.
- Delivery status is visible to the user where appropriate.
- Moderation and security events retain their audit record after notification deletion.

## 12. Optional SMTP adapter

### 12.1 Configuration

SMTP settings are supplied outside the repository:

- host;
- port;
- encryption mode;
- username;
- password or secret reference;
- from address;
- reply-to address;
- connection timeout;
- maximum messages per connection;
- daily send limit.

No SMTP credential is committed or placed in a public configuration file.

### 12.2 Delivery behavior

- Use an outbox/queue record rather than sending synchronously in a request.
- Send in the background.
- Use idempotency keys.
- Retry transient failures with backoff.
- Do not retry permanent failures such as invalid recipients indefinitely.
- Record message ID, provider response, attempt count, and final status.
- Respect the configured rate limit.
- Use plain text and approved HTML templates.
- Do not embed private application data in the subject line.
- Support an email preview for authorized organizers.

### 12.3 Local fallback

When SMTP is not configured:

- in-app notifications still work;
- email rows remain in an `outbox` state such as `not_configured` or `queued_local`;
- no network call is made;
- the application starts normally;
- the acceptance checker is unaffected.

## 13. Audit and security event model

Every security-relevant action produces an immutable audit event with:

- actor account and role;
- target type and ID;
- event scope;
- action name;
- before/after safe summary;
- reason;
- IP/network metadata subject to privacy policy;
- user-agent/device metadata;
- request/correlation ID;
- authentication method;
- timestamp.

Audit categories:

- authentication;
- session;
- account state;
- role and permission;
- team membership;
- judge assignment;
- review and result alteration;
- import/export;
- integration;
- moderation;
- deletion and retention.

Audit logs are append-only from the application perspective. Corrections create a compensating event; they do not overwrite history.

## 14. Data model additions

The initial migration already includes core account and audit tables. The account-management work adds or refines:

- `account_states`;
- `sessions` with hashed tokens, expiry, device metadata, and revocation;
- `account_merge_records`;
- `account_recovery_actions`;
- `team_memberships` with member/leader/captain state;
- `team_invites` with hashed, expiring tokens;
- `team_deletion_requests`;
- `event_role_assignments`;
- `notification_preferences`;
- `notification_deliveries`;
- `email_outbox`;
- `security_events`;
- `permission_overrides` with expiry and reason, if explicitly supported.

All records are event-scoped or explicitly global. Sensitive fields are encrypted or hashed according to the data classification.

## 15. API surface for account management

Exact paths are implementation decisions, but the API must cover these operations:

### Current account

- get current profile;
- update current profile;
- change password;
- list active sessions;
- revoke one session;
- revoke all sessions;
- update email or username;
- update notification preferences;
- export own account data;
- request account deletion.

### Teams

- create team;
- view team;
- update team metadata;
- list members and roles;
- invite member;
- revoke invite;
- accept/decline invite;
- remove member;
- promote member;
- demote leader;
- transfer captaincy;
- leave team;
- request team deletion;
- archive team.

### Administration

- search accounts;
- view account detail;
- change account state;
- assign/revoke global role;
- assign/revoke event role;
- revoke user sessions;
- merge accounts;
- view security events;
- export audit data;
- manage API tokens and SMTP configuration status.

Every endpoint requires a documented permission and emits an audit event for protected actions.

## 16. Implementation phases

### P0 — Local account foundation

- local login;
- server-side session records;
- account states;
- role permissions;
- session revocation;
- local development identities.

### P1 — Team identity

- team membership roles;
- captain/leader lifecycle;
- invitations;
- promotion/demotion;
- transfer and deletion safeguards;
- team-scoped authorization tests.

### P2 — Organizer and admin

- event role assignment;
- account search and state management;
- session revocation by organizer/admin;
- audit viewer;
- API tokens.

### P3 — Notifications

- in-app inbox;
- event catalog;
- preferences;
- digests;
- idempotent delivery records;
- moderation/security notice safety.

### P4 — Account center completion

- password change and recovery;
- account data export;
- account deletion and retention workflow;
- account merge workflow;
- notification preferences and digests;
- optional SMTP outbox and delivery worker;
- webhook delivery;
- signed email/verification links.

Optional mail transport is never allowed to become a prerequisite for the local acceptance path.

## 17. Security acceptance tests

The implementation should test at least:

- a participant cannot read a peer private team record;
- a member cannot promote themselves;
- a leader cannot demote the captain;
- a leader cannot delete a team;
- a captain transfer is atomic;
- a removed member loses private team access;
- a suspended account cannot authenticate;
- a revoked session is rejected;
- a password change revokes other sessions;
- a judge cannot read peer scores;
- a judge cannot access an unassigned track;
- a participant cannot access organizer exports;
- an organizer cannot access another event without a grant;
- a non-admin cannot assign global roles;
- an email or username change cannot merge accounts;
- SMTP absence does not block local startup;
- a notification never contains private judge data;
- every protected mutation produces an audit event.

## 18. Current handoff

- License: MIT in `LICENSE`.
- Local first-party auth: required and only identity system.
- External identity providers: not supported and not planned.
- SMTP: optional mail transport for notifications only, disabled unless configured.
- In-app notifications: required default.
- Team roles: member, leader, captain.
- Global roles: participant, judge, organizer, admin.
- Service identities: explicit and non-human.
- Permissions: deny by default and evaluated server-side.
- Current implementation: backend has local bcrypt auth, store-backed revocable sessions, profile and password endpoints, account export, deletion request/cancel lifecycle, account/session management, team membership roles, promotion/demotion/captaincy transfer, team archive safeguards, in-app notifications, admin account state/role endpoints, audit records, and first-party My Account boundaries.
- Still pending: account recovery, PostgreSQL runtime wiring for the new tables, SMTP outbox worker, notification preferences, account merge/anonymization, and durable export jobs.
- Next implementation slice: add account recovery and PostgreSQL-backed account/session persistence.

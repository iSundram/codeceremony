-- CodeCeremony hackathon hosting expansion: custom questions, profiles, team
-- formation, judge rosters, leaderboards, and participation history.

CREATE TABLE IF NOT EXISTS hackathon_questions (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    prompt TEXT NOT NULL,
    help_text TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL CHECK (type IN ('short_text', 'long_text', 'url', 'select', 'checkbox', 'number')),
    audience TEXT NOT NULL CHECK (audience IN ('submission', 'team', 'participant')),
    required BOOLEAN NOT NULL DEFAULT FALSE,
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (event_id, key, audience)
);

CREATE INDEX IF NOT EXISTS hackathon_questions_event_idx
    ON hackathon_questions (event_id, audience, position);

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'draft',
    ADD COLUMN IF NOT EXISTS judging_mode TEXT NOT NULL DEFAULT 'automatic',
    ADD COLUMN IF NOT EXISTS judging_close_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS min_team_size INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS max_team_size INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS allow_global_teams BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS reviews_per_project INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS leaderboard_public BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS results_published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE events
    ADD CONSTRAINT events_state_check CHECK (state IN ('draft', 'registration_open', 'submissions_open', 'submissions_closed', 'judging', 'results_published', 'archived')),
    ADD CONSTRAINT events_judging_mode_check CHECK (judging_mode IN ('automatic', 'manual')),
    ADD CONSTRAINT events_reviews_per_project_check CHECK (reviews_per_project >= 1 AND reviews_per_project <= 10),
    ADD CONSTRAINT events_team_size_check CHECK (min_team_size >= 1 AND max_team_size >= min_team_size);

CREATE TABLE IF NOT EXISTS hackathon_milestones (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    due_at TIMESTAMPTZ,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS hackathon_hosts (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    headline TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    links JSONB NOT NULL DEFAULT '{}'::jsonb,
    availability TEXT NOT NULL DEFAULT 'solo'
        CHECK (availability IN ('solo', 'looking_for_team', 'teamed')),
    open_to_invites BOOLEAN NOT NULL DEFAULT TRUE,
    seeking_team BOOLEAN NOT NULL DEFAULT FALSE,
    seeking_role TEXT NOT NULL DEFAULT '',
    seeking_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT user_profiles_availability_check CHECK (NOT (availability = 'teamed' AND seeking_team))
);

CREATE INDEX IF NOT EXISTS user_profiles_seeking_idx
    ON user_profiles (availability, seeking_event_id)
    WHERE seeking_team;

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'hackathon',
    ADD COLUMN IF NOT EXISTS availability TEXT NOT NULL DEFAULT 'invite_only',
    ADD COLUMN IF NOT EXISTS max_size INTEGER,
    ADD COLUMN IF NOT EXISTS open_roles JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE teams
    ADD CONSTRAINT teams_scope_check CHECK (scope IN ('hackathon', 'global')),
    ADD CONSTRAINT teams_availability_check CHECK (availability IN ('open', 'invite_only', 'closed', 'full')),
    ADD CONSTRAINT teams_max_size_check CHECK (max_size IS NULL OR max_size >= 1);

CREATE TABLE IF NOT EXISTS team_invites (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    event_id TEXT REFERENCES events(id) ON DELETE CASCADE,
    inviter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    invitee_email TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'member',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'revoked', 'expired')),
    message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    CHECK (invitee_id IS NOT NULL OR invitee_email <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS team_invites_pending_unique
    ON team_invites (team_id, COALESCE(invitee_id, invitee_email))
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS team_invites_inbox_idx
    ON team_invites (invitee_id, invitee_email, status);

CREATE TABLE IF NOT EXISTS hackathon_judge_roster (
    event_id TEXT NOT NULL DEFAULT '',
    judge_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('hackathon', 'global')),
    headline TEXT NOT NULL DEFAULT '',
    expertise JSONB NOT NULL DEFAULT '[]'::jsonb,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    added_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (event_id, judge_id)
);

CREATE TABLE IF NOT EXISTS result_publications (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    published_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    is_public BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    pending_reviews INTEGER NOT NULL DEFAULT 0,
    notified INTEGER NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NOT NULL,
    unpublished_at TIMESTAMPTZ,
    unpublish_reason TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS participations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    team_id TEXT REFERENCES teams(id) ON DELETE SET NULL,
    role TEXT NOT NULL CHECK (role IN ('captain', 'member', 'judge', 'organizer')),
    project_id TEXT REFERENCES submissions(id) ON DELETE SET NULL,
    result TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, event_id)
);

CREATE INDEX IF NOT EXISTS participations_event_idx
    ON participations (event_id, role);

ALTER TABLE submissions
    ADD COLUMN IF NOT EXISTS story TEXT NOT NULL DEFAULT '';

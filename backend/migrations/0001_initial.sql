CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('participant', 'judge', 'organizer', 'admin')),
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    state TEXT NOT NULL CHECK (state IN ('draft', 'registration_open', 'submissions_open', 'submissions_closed', 'judging', 'voting', 'results_published', 'archived')),
    registration_opens_at TIMESTAMPTZ,
    submissions_open_at TIMESTAMPTZ,
    submissions_close_at TIMESTAMPTZ NOT NULL,
    judging_opens_at TIMESTAMPTZ,
    judging_close_at TIMESTAMPTZ,
    voting_opens_at TIMESTAMPTZ,
    voting_close_at TIMESTAMPTZ,
    results_published_at TIMESTAMPTZ,
    results_visible BOOLEAN NOT NULL DEFAULT FALSE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS event_organizers (
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, user_id)
);

CREATE TABLE IF NOT EXISTS tracks (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    UNIQUE (event_id, slug)
);

CREATE TABLE IF NOT EXISTS prizes (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    rank INTEGER,
    value TEXT
);

CREATE TABLE IF NOT EXISTS custom_questions (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    prompt TEXT NOT NULL,
    response_type TEXT NOT NULL CHECK (response_type IN ('text', 'long_text', 'single_select', 'multi_select', 'boolean')),
    required BOOLEAN NOT NULL DEFAULT FALSE,
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    visible_to_judges BOOLEAN NOT NULL DEFAULT TRUE,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    captain_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    eligibility_status TEXT NOT NULL DEFAULT 'pending' CHECK (eligibility_status IN ('pending', 'eligible', 'ineligible')),
    eligibility_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    membership_status TEXT NOT NULL DEFAULT 'active' CHECK (membership_status IN ('active', 'left', 'removed')),
    joined_at TIMESTAMPTZ NOT NULL,
    left_at TIMESTAMPTZ,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE IF NOT EXISTS team_invites (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    max_uses INTEGER NOT NULL DEFAULT 1,
    uses INTEGER NOT NULL DEFAULT 0,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS submissions (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    thumbnail_url TEXT,
    video_url TEXT,
    repository_url TEXT,
    live_url TEXT,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('draft', 'submitted', 'needs_changes', 'withdrawn', 'locked', 'disqualified')),
    submitted_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS submission_versions (
    id TEXT PRIMARY KEY,
    submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    payload JSONB NOT NULL,
    change_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (submission_id, version)
);

CREATE TABLE IF NOT EXISTS submission_answers (
    submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    question_id TEXT NOT NULL REFERENCES custom_questions(id) ON DELETE RESTRICT,
    value JSONB NOT NULL,
    PRIMARY KEY (submission_id, question_id)
);

CREATE TABLE IF NOT EXISTS rubrics (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    active_version_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rubric_versions (
    id TEXT PRIMARY KEY,
    rubric_id TEXT NOT NULL REFERENCES rubrics(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'retired')),
    scale_min INTEGER NOT NULL DEFAULT 1,
    scale_max INTEGER NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (rubric_id, version)
);

CREATE TABLE IF NOT EXISTS criteria (
    id TEXT PRIMARY KEY,
    rubric_version_id TEXT NOT NULL REFERENCES rubric_versions(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    label TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    weight NUMERIC(7, 4) NOT NULL CHECK (weight > 0),
    required BOOLEAN NOT NULL DEFAULT TRUE,
    comment_required BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL DEFAULT 0,
    UNIQUE (rubric_version_id, key)
);

CREATE TABLE IF NOT EXISTS judge_profiles (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bio TEXT NOT NULL DEFAULT '',
    capacity INTEGER,
    active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS judge_track_scopes (
    judge_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    PRIMARY KEY (judge_id, track_id)
);

CREATE TABLE IF NOT EXISTS conflict_declarations (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    judge_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id TEXT REFERENCES teams(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES submissions(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (team_id IS NOT NULL OR project_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS assignments (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    judge_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    assigned_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    UNIQUE (event_id, judge_id, project_id)
);

CREATE TABLE IF NOT EXISTS reviews (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    judge_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    assignment_id TEXT REFERENCES assignments(id) ON DELETE SET NULL,
    rubric_version_id TEXT NOT NULL REFERENCES rubric_versions(id) ON DELETE RESTRICT,
    comment TEXT NOT NULL DEFAULT '',
    private_note TEXT NOT NULL DEFAULT '',
    submitted BOOLEAN NOT NULL DEFAULT FALSE,
    submitted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (event_id, judge_id, project_id)
);

CREATE TABLE IF NOT EXISTS review_scores (
    review_id TEXT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    criterion_id TEXT NOT NULL REFERENCES criteria(id) ON DELETE RESTRICT,
    score INTEGER NOT NULL CHECK (score >= 1 AND score <= 5),
    PRIMARY KEY (review_id, criterion_id)
);

CREATE TABLE IF NOT EXISTS normalization_runs (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    method_version TEXT NOT NULL,
    weights JSONB NOT NULL,
    input_hash TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS normalized_scores (
    normalization_run_id TEXT NOT NULL REFERENCES normalization_runs(id) ON DELETE CASCADE,
    review_id TEXT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    raw_score NUMERIC(12, 6) NOT NULL,
    normalized_score NUMERIC(12, 6) NOT NULL,
    low_information BOOLEAN NOT NULL DEFAULT FALSE,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (normalization_run_id, review_id)
);

CREATE TABLE IF NOT EXISTS result_snapshots (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    normalization_run_id TEXT NOT NULL REFERENCES normalization_runs(id) ON DELETE RESTRICT,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS voting_campaigns (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('disabled', 'open_link', 'email_token', 'authenticated')),
    method TEXT NOT NULL CHECK (method IN ('one_per_person', 'approval', 'quadratic')),
    opens_at TIMESTAMPTZ,
    closes_at TIMESTAMPTZ,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS ballots (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES voting_campaigns(id) ON DELETE CASCADE,
    voter_key TEXT NOT NULL,
    token_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    submitted_at TIMESTAMPTZ,
    UNIQUE (campaign_id, voter_key)
);

CREATE TABLE IF NOT EXISTS votes (
    id TEXT PRIMARY KEY,
    ballot_id TEXT NOT NULL REFERENCES ballots(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    weight NUMERIC(12, 6) NOT NULL DEFAULT 1,
    invalidated_at TIMESTAMPTZ,
    invalidation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    author_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'visible' CHECK (status IN ('visible', 'pending', 'removed')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    removed_at TIMESTAMPTZ,
    removal_reason TEXT
);

CREATE TABLE IF NOT EXISTS certificates (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    recipient_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    recipient_team_id TEXT REFERENCES teams(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    verification_code TEXT NOT NULL UNIQUE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    issued_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    events JSONB NOT NULL DEFAULT '[]'::jsonb,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_name TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    response_status INTEGER,
    delivered_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    error TEXT
);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id TEXT REFERENCES events(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS import_jobs (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    format TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    error_report JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS export_jobs (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    format TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    storage_path TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
    actor_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT,
    reason TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_submissions_event_status ON submissions (event_id, status);
CREATE INDEX IF NOT EXISTS idx_submissions_team ON submissions (team_id);
CREATE INDEX IF NOT EXISTS idx_reviews_event_judge ON reviews (event_id, judge_id);
CREATE INDEX IF NOT EXISTS idx_reviews_project ON reviews (project_id);
CREATE INDEX IF NOT EXISTS idx_assignments_event_judge ON assignments (event_id, judge_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_event_created ON audit_events (event_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_comments_project_created ON comments (project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications (user_id, created_at DESC);

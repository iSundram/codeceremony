-- CodeCeremony activity log, event staff roles, and mail delivery.

CREATE TABLE IF NOT EXISTS activity_entries (
    id TEXT PRIMARY KEY,
    event_id TEXT REFERENCES events(id) ON DELETE CASCADE,
    actor_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    actor_name TEXT NOT NULL DEFAULT '',
    actor_role TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL CHECK (category IN ('event', 'team', 'submission', 'judging', 'results', 'account', 'communication')),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL,
    visibility TEXT NOT NULL CHECK (visibility IN ('public', 'participants', 'judges', 'organizers')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS activity_entries_event_idx
    ON activity_entries (event_id, created_at DESC);

CREATE INDEX IF NOT EXISTS activity_entries_category_idx
    ON activity_entries (event_id, category, created_at DESC);

CREATE INDEX IF NOT EXISTS activity_entries_visibility_idx
    ON activity_entries (visibility, created_at DESC);

CREATE TABLE IF NOT EXISTS event_staff (
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'co_organizer', 'judge_liaison', 'viewer')),
    title TEXT NOT NULL DEFAULT '',
    added_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX IF NOT EXISTS event_staff_role_idx
    ON event_staff (event_id, role);

CREATE TABLE IF NOT EXISTS mail_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    transactional BOOLEAN NOT NULL DEFAULT TRUE,
    account_security BOOLEAN NOT NULL DEFAULT TRUE,
    account_lifecycle BOOLEAN NOT NULL DEFAULT TRUE,
    team_activity BOOLEAN NOT NULL DEFAULT TRUE,
    event_activity BOOLEAN NOT NULL DEFAULT TRUE,
    judging BOOLEAN NOT NULL DEFAULT TRUE,
    results BOOLEAN NOT NULL DEFAULT TRUE,
    marketing BOOLEAN NOT NULL DEFAULT FALSE,
    digest_only BOOLEAN NOT NULL DEFAULT FALSE,
    weekly_digest BOOLEAN NOT NULL DEFAULT FALSE,
    unsubscribed_all BOOLEAN NOT NULL DEFAULT FALSE,
    unsubscribed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT mail_preferences_security_check CHECK (transactional OR (NOT account_security AND NOT account_lifecycle))
);

CREATE TABLE IF NOT EXISTS mail_messages (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
    template TEXT NOT NULL,
    topic TEXT NOT NULL CHECK (topic IN ('account_security', 'account_lifecycle', 'team_activity', 'event_activity', 'judging', 'results', 'marketing')),
    kind TEXT NOT NULL CHECK (kind IN ('transactional', 'promotional')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'skipped', 'cancelled')),
    subject TEXT NOT NULL,
    body_text TEXT NOT NULL,
    body_html TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    dedupe_key TEXT NOT NULL DEFAULT '',
    scheduled_at TIMESTAMPTZ NOT NULL,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS mail_messages_dedupe_unique
    ON mail_messages (dedupe_key)
    WHERE dedupe_key <> '';

CREATE INDEX IF NOT EXISTS mail_messages_queue_idx
    ON mail_messages (status, scheduled_at);

CREATE TABLE IF NOT EXISTS unsubscribe_tokens (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope TEXT NOT NULL DEFAULT 'marketing',
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS unsubscribe_tokens_user_idx
    ON unsubscribe_tokens (user_id, scope);

-- CodeCeremony community layer: comments, moderation, voting, and webhooks.

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    author_name TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL,
    parent_id TEXT REFERENCES comments(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'visible' CHECK (status IN ('visible', 'hidden', 'deleted')),
    moderated_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    moderation_note TEXT NOT NULL DEFAULT '',
    moderated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX IF NOT EXISTS comments_project_idx
    ON comments (project_id, created_at);

CREATE INDEX IF NOT EXISTS comments_visible_idx
    ON comments (project_id)
    WHERE status = 'visible';

CREATE TABLE IF NOT EXISTS comment_reports (
    id TEXT PRIMARY KEY,
    comment_id TEXT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'actioned', 'dismissed')),
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    resolution_note TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS comment_reports_open_unique
    ON comment_reports (comment_id, reporter_id)
    WHERE status = 'open';

CREATE TABLE IF NOT EXISTS vote_campaigns (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'closed')),
    max_choices_per_user INTEGER NOT NULL DEFAULT 3 CHECK (max_choices_per_user BETWEEN 1 AND 10),
    opens_at TIMESTAMPTZ,
    closes_at TIMESTAMPTZ,
    require_eligible BOOLEAN NOT NULL DEFAULT FALSE,
    created_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS vote_campaigns_event_idx
    ON vote_campaigns (event_id, status);

CREATE TABLE IF NOT EXISTS ballots (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES vote_campaigns(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (campaign_id, user_id, project_id)
);

CREATE INDEX IF NOT EXISTS ballots_tally_idx
    ON ballots (campaign_id, project_id);

CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    events JSONB NOT NULL DEFAULT '[]'::jsonb,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    last_delivery_at TIMESTAMPTZ,
    failure_count INTEGER NOT NULL DEFAULT 0,
    delivery_count INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS webhooks_event_idx
    ON webhooks (event_id, active);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id TEXT,
    event TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    response_code INTEGER,
    response_body TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ,
    next_attempt TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS webhook_deliveries_queue_idx
    ON webhook_deliveries (status, next_attempt);

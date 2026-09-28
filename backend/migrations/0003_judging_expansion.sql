-- CodeCeremony judging and submission lifecycle expansion.
-- Aligns the relational schema with the in-memory domain model introduced in the
-- assignment builder, rubric versioning, and submission lifecycle slices.

ALTER TABLE assignments
    ADD COLUMN IF NOT EXISTS strategy TEXT NOT NULL DEFAULT 'balanced'
        CHECK (strategy IN ('manual', 'balanced', 'batch'));

CREATE UNIQUE INDEX IF NOT EXISTS assignments_active_unique
    ON assignments (event_id, judge_id, project_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS assignments_event_active_idx
    ON assignments (event_id, revoked_at);

CREATE INDEX IF NOT EXISTS conflict_declarations_lookup_idx
    ON conflict_declarations (event_id, judge_id, project_id);

-- Rubric criteria are now version-scoped and allow per-criterion score ranges.
ALTER TABLE criteria
    ADD COLUMN IF NOT EXISTS min_score INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS max_score INTEGER NOT NULL DEFAULT 5;

ALTER TABLE review_scores
    DROP CONSTRAINT IF EXISTS review_scores_score_check;
ALTER TABLE review_scores
    ADD CONSTRAINT review_scores_score_check CHECK (score >= 0 AND score <= 10);

ALTER TABLE rubric_versions
    ADD COLUMN IF NOT EXISTS track_id TEXT REFERENCES tracks(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS instructions TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

ALTER TABLE rubric_versions
    DROP CONSTRAINT IF EXISTS rubric_versions_status_check;
ALTER TABLE rubric_versions
    ADD CONSTRAINT rubric_versions_status_check CHECK (status IN ('draft', 'published', 'active', 'archived', 'retired'));

CREATE INDEX IF NOT EXISTS rubric_versions_active_idx
    ON rubric_versions (rubric_id, track_id, version DESC)
    WHERE status IN ('published', 'active');

-- reviews carry the rubric they were scored against plus a normalized projection.
ALTER TABLE reviews
    ADD COLUMN IF NOT EXISTS rubric_id TEXT REFERENCES rubrics(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS rubric_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS normalized JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Submission eligibility review and version history metadata.
ALTER TABLE submissions
    ADD COLUMN IF NOT EXISTS eligibility TEXT NOT NULL DEFAULT 'pending'
        CHECK (eligibility IN ('pending', 'eligible', 'ineligible')),
    ADD COLUMN IF NOT EXISTS eligibility_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS eligibility_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS eligibility_at TIMESTAMPTZ;

ALTER TABLE submission_versions
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';

-- Duplicate detection queue.
CREATE TABLE IF NOT EXISTS duplicate_flags (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    duplicate_of_project_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    signal TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'confirmed', 'dismissed')),
    resolution_note TEXT NOT NULL DEFAULT '',
    resolved_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    CHECK (project_id <> duplicate_of_project_id),
    UNIQUE (event_id, project_id, duplicate_of_project_id)
);

CREATE INDEX IF NOT EXISTS duplicate_flags_open_idx
    ON duplicate_flags (event_id, status);

-- Judge profile capacity is optional in the model but constrained when present.
ALTER TABLE judge_profiles
    DROP CONSTRAINT IF EXISTS judge_profiles_capacity_check;
ALTER TABLE judge_profiles
    ADD CONSTRAINT judge_profiles_capacity_check CHECK (capacity IS NULL OR capacity >= 0);

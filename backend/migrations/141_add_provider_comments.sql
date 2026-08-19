-- 141_add_provider_comments.sql
CREATE TABLE IF NOT EXISTS provider_comments (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    sentiment VARCHAR(20) NOT NULL DEFAULT 'positive',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    like_count INTEGER NOT NULL DEFAULT 0,
    dislike_count INTEGER NOT NULL DEFAULT 0,
    reply_count INTEGER NOT NULL DEFAULT 0,
    parent_id BIGINT REFERENCES provider_comments(id) ON DELETE SET NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_provider_comments_provider_status_created_at
    ON provider_comments (provider, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_provider_comments_provider_sentiment_status
    ON provider_comments (provider, sentiment, status);

CREATE INDEX IF NOT EXISTS idx_provider_comments_parent_id
    ON provider_comments (parent_id);

CREATE INDEX IF NOT EXISTS idx_provider_comments_user_id
    ON provider_comments (user_id);

CREATE TABLE IF NOT EXISTS provider_comment_votes (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    vote_type VARCHAR(20) NOT NULL,
    comment_id BIGINT NOT NULL REFERENCES provider_comments(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT provider_comment_votes_comment_user_key UNIQUE (comment_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_provider_comment_votes_comment_user
    ON provider_comment_votes (comment_id, user_id);

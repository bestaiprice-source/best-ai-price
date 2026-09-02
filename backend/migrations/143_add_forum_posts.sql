-- 143_add_forum_posts.sql
CREATE TABLE IF NOT EXISTS forum_posts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    like_count INTEGER NOT NULL DEFAULT 0,
    dislike_count INTEGER NOT NULL DEFAULT 0,
    reply_count INTEGER NOT NULL DEFAULT 0,
    parent_id BIGINT REFERENCES forum_posts(id) ON DELETE SET NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_forum_posts_status_created_at
    ON forum_posts (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_forum_posts_parent_id
    ON forum_posts (parent_id);

CREATE INDEX IF NOT EXISTS idx_forum_posts_user_id
    ON forum_posts (user_id);

CREATE TABLE IF NOT EXISTS forum_post_votes (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    vote_type VARCHAR(20) NOT NULL,
    post_id BIGINT NOT NULL REFERENCES forum_posts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT forum_post_votes_post_user_key UNIQUE (post_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_forum_post_votes_post_user
    ON forum_post_votes (post_id, user_id);

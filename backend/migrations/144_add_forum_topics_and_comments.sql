-- 144_add_forum_topics_and_comments.sql

-- Drop legacy forum_posts / forum_post_votes if exists
DROP TABLE IF EXISTS forum_post_votes CASCADE;
DROP TABLE IF EXISTS forum_posts CASCADE;

CREATE TABLE IF NOT EXISTS forum_topics (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    like_count INTEGER NOT NULL DEFAULT 0,
    dislike_count INTEGER NOT NULL DEFAULT 0,
    comment_count INTEGER NOT NULL DEFAULT 0,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_forum_topics_status_created_at
    ON forum_topics (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_forum_topics_user_id
    ON forum_topics (user_id);

CREATE TABLE IF NOT EXISTS forum_topic_votes (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    vote_type VARCHAR(20) NOT NULL,
    topic_id BIGINT NOT NULL REFERENCES forum_topics(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT forum_topic_votes_topic_user_key UNIQUE (topic_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_forum_topic_votes_topic_user
    ON forum_topic_votes (topic_id, user_id);

CREATE TABLE IF NOT EXISTS forum_comments (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    like_count INTEGER NOT NULL DEFAULT 0,
    dislike_count INTEGER NOT NULL DEFAULT 0,
    reply_count INTEGER NOT NULL DEFAULT 0,
    topic_id BIGINT NOT NULL REFERENCES forum_topics(id) ON DELETE CASCADE,
    parent_id BIGINT REFERENCES forum_comments(id) ON DELETE SET NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_forum_comments_topic_status_created_at
    ON forum_comments (topic_id, status, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_forum_comments_parent_id
    ON forum_comments (parent_id);

CREATE INDEX IF NOT EXISTS idx_forum_comments_user_id
    ON forum_comments (user_id);

CREATE TABLE IF NOT EXISTS forum_comment_votes (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    vote_type VARCHAR(20) NOT NULL,
    comment_id BIGINT NOT NULL REFERENCES forum_comments(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT forum_comment_votes_comment_user_key UNIQUE (comment_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_forum_comment_votes_comment_user
    ON forum_comment_votes (comment_id, user_id);

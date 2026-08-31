-- 142_add_provider_popularity.sql
CREATE TABLE IF NOT EXISTS provider_popularity (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider VARCHAR(100) NOT NULL,
    click_count BIGINT NOT NULL DEFAULT 0,
    last_clicked_at TIMESTAMPTZ,
    CONSTRAINT uq_provider_popularity_provider UNIQUE (provider)
);

CREATE INDEX IF NOT EXISTS idx_provider_popularity_click_count
    ON provider_popularity (click_count DESC);

CREATE INDEX IF NOT EXISTS idx_provider_popularity_provider
    ON provider_popularity (provider);

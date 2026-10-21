-- 151_add_support_messages.sql
-- Two-way support chat (§15): user messages are forwarded to the admin Telegram
-- bot, and admin replies (Telegram "reply" on the forwarded message) are stored
-- here so the user sees them in the support widget.
-- Additive only: no existing table is touched.

CREATE TABLE IF NOT EXISTS support_messages (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT       NOT NULL,
    direction   VARCHAR(8)   NOT NULL, -- 'user' | 'admin'
    body        TEXT         NOT NULL,
    request_id  VARCHAR(128) NOT NULL DEFAULT '',
    model       VARCHAR(128) NOT NULL DEFAULT '',
    page        VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    read_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_support_messages_user
    ON support_messages(user_id, id);

-- Maps every Telegram message we sent (one per recipient chat) back to the
-- support message, so a reply to it can be routed to the right user.
CREATE TABLE IF NOT EXISTS support_telegram_links (
    chat_id            BIGINT NOT NULL,
    tg_message_id      BIGINT NOT NULL,
    support_message_id BIGINT NOT NULL REFERENCES support_messages(id) ON DELETE CASCADE,
    PRIMARY KEY (chat_id, tg_message_id)
);

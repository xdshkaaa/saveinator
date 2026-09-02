CREATE TYPE language AS ENUM ('EN', 'RU', 'KK');
CREATE TYPE platform AS ENUM ('YOUTUBE', 'TIKTOK', 'INSTAGRAM', 'X', 'SPOTIFY', 'SOUNDCLOUD', 'PINTEREST', 'UNKNOWN');
CREATE TYPE downloadstatus AS ENUM ('QUEUED', 'FETCHING_FORMATS', 'DOWNLOADING', 'TRANSCODING', 'SENDING', 'COMPLETED', 'FAILED');
CREATE TYPE broadcaststatus AS ENUM ('DRAFT', 'QUEUED', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED');
CREATE TYPE broadcastaudience AS ENUM ('ALL', 'RU', 'EN', 'ACTIVE');
CREATE TYPE broadcastdeliverystatus AS ENUM ('PENDING', 'SENT', 'FAILED', 'BLOCKED');

CREATE TABLE bots (
    slug VARCHAR(32) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

INSERT INTO bots (slug) VALUES ('saveinator'), ('pinterest'), ('pinterest_kz'), ('spotify');

CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    username VARCHAR(64),
    first_name VARCHAR(128),
    language language NOT NULL DEFAULT 'EN',
    bot_id VARCHAR(32) REFERENCES bots(slug),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE user_bot_settings (
    user_id BIGINT NOT NULL REFERENCES users(id),
    bot_id VARCHAR(32) NOT NULL REFERENCES bots(slug),
    language language NOT NULL DEFAULT 'EN',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, bot_id)
);

CREATE TABLE chats (
    id BIGINT PRIMARY KEY,
    title VARCHAR(255),
    type VARCHAR(16) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE downloads (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    chat_id BIGINT NOT NULL REFERENCES chats(id),
    url TEXT NOT NULL,
    platform platform NOT NULL,
    format_id VARCHAR(64),
    quality_label VARCHAR(32),
    file_size BIGINT,
    status downloadstatus NOT NULL DEFAULT 'QUEUED',
    bot_id VARCHAR(32) REFERENCES bots(slug),
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE TABLE user_settings (
    user_id BIGINT PRIMARY KEY REFERENCES users(id),
    youtube_quality VARCHAR(16) NOT NULL DEFAULT 'ask',
    youtube_ratio VARCHAR(16) NOT NULL DEFAULT 'ask',
    no_watermark BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE purchases (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    product VARCHAR(32) NOT NULL,
    stars_amount INT NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'XTR',
    telegram_payment_charge_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX ix_purchases_user ON purchases (user_id, product);

CREATE TABLE banned_links (
    id SERIAL PRIMARY KEY,
    url_hash VARCHAR(64) UNIQUE NOT NULL,
    reason VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE broadcasts (
    id SERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    text TEXT NOT NULL,
    audience broadcastaudience NOT NULL DEFAULT 'ALL',
    status broadcaststatus NOT NULL DEFAULT 'DRAFT',
    total_recipients INTEGER NOT NULL DEFAULT 0,
    sent_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    blocked_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE TABLE broadcast_deliveries (
    id SERIAL PRIMARY KEY,
    broadcast_id INTEGER NOT NULL REFERENCES broadcasts(id),
    user_id BIGINT NOT NULL,
    status broadcastdeliverystatus NOT NULL DEFAULT 'PENDING',
    error_message TEXT,
    sent_at TIMESTAMP
);

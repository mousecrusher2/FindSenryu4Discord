CREATE TABLE IF NOT EXISTS senryus (
    id SERIAL PRIMARY KEY,
    server_id TEXT,
    author_id TEXT,
    kamigo TEXT,
    nakasichi TEXT,
    simogo TEXT,
    spoiler BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_senryus_server_id ON senryus(server_id);
CREATE INDEX IF NOT EXISTS idx_senryus_author_id ON senryus(author_id);
CREATE INDEX IF NOT EXISTS idx_senryus_server_spoiler ON senryus(server_id, spoiler);

CREATE TABLE IF NOT EXISTS muted_channels (
    channel_id TEXT PRIMARY KEY,
    guild_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_muted_channels_guild_id ON muted_channels(guild_id);

CREATE TABLE IF NOT EXISTS guild_channel_type_settings (
    guild_id TEXT,
    channel_type INTEGER,
    enabled BOOLEAN,
    PRIMARY KEY (guild_id, channel_type)
);

CREATE TABLE IF NOT EXISTS detection_opt_outs (
    server_id TEXT,
    user_id TEXT,
    PRIMARY KEY (server_id, user_id)
);

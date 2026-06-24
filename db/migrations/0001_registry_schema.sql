-- Robot Agent Registry — Phase 1 schema (spec §6)
-- Plain PostgreSQL; no Supabase-specific extensions required so this can be
-- self-hosted against any Postgres 14+.

-- Owner（所有者）
CREATE TABLE IF NOT EXISTS owners (
    owner_id     TEXT PRIMARY KEY,          -- "usr_<nanoid>"
    public_key   TEXT NOT NULL UNIQUE,      -- Ed25519 hex
    display_name TEXT NOT NULL,
    email        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Platform（硬件平台）
CREATE TABLE IF NOT EXISTS platforms (
    platform_id        TEXT PRIMARY KEY,    -- "plt_<nanoid>"
    owner_id           TEXT NOT NULL REFERENCES owners(owner_id) ON DELETE CASCADE,
    platform_type      TEXT NOT NULL,
    display_name       TEXT NOT NULL,
    description        TEXT,
    platform_token_hash TEXT NOT NULL,      -- bcrypt hash; plaintext returned only once at creation
    public_key         TEXT NOT NULL,       -- platform Ed25519 public key (hex)
    last_seen_at       TIMESTAMPTZ,
    online             BOOLEAN NOT NULL DEFAULT FALSE,
    tags               TEXT[] NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent（软件 Agent）
CREATE TABLE IF NOT EXISTS agents (
    agent_id       TEXT PRIMARY KEY,        -- "agt_<nanoid>"
    platform_id    TEXT NOT NULL REFERENCES platforms(platform_id) ON DELETE CASCADE,
    owner_id       TEXT NOT NULL REFERENCES owners(owner_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    description    TEXT,
    agent_type     TEXT NOT NULL,
    version        TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'offline',   -- online|offline|error
    visibility     TEXT NOT NULL DEFAULT 'public',    -- public|private|unlisted
    tunnel_endpoint TEXT,
    mcp_endpoint   TEXT,
    rest_endpoint  TEXT,
    metadata       JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform_id, name)
);

-- Capability（能力/服务）
CREATE TABLE IF NOT EXISTS capabilities (
    capability_id  TEXT PRIMARY KEY,        -- "cap_<nanoid>"
    agent_id       TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    description    TEXT,
    interface_type TEXT NOT NULL,           -- mcp_tool|rest|ros2_service|ros2_action|ros2_topic
    permission     TEXT NOT NULL DEFAULT 'public', -- public|authenticated|paid|owner_only|authorized_list
    pricing        JSONB,
    authorized_agents TEXT[] NOT NULL DEFAULT '{}',
    input_schema   JSONB NOT NULL DEFAULT '{}',
    output_schema  JSONB NOT NULL DEFAULT '{}',
    ros2_metadata  JSONB,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, name)
);

-- Config（owner 下发给 platform 的配置；Phase 2 才会有 WebSocket 推送，这里先建表）
CREATE TABLE IF NOT EXISTS platform_configs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    platform_id     TEXT NOT NULL REFERENCES platforms(platform_id) ON DELETE CASCADE,
    target_agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE,
    key             TEXT NOT NULL,
    value           TEXT NOT NULL,
    issued_by       TEXT NOT NULL,          -- owner_id
    applied_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agents_platform        ON agents(platform_id);
CREATE INDEX IF NOT EXISTS idx_agents_visibility      ON agents(visibility, status);
CREATE INDEX IF NOT EXISTS idx_capabilities_agent     ON capabilities(agent_id);
CREATE INDEX IF NOT EXISTS idx_capabilities_permission ON capabilities(permission);
CREATE INDEX IF NOT EXISTS idx_platforms_owner        ON platforms(owner_id);

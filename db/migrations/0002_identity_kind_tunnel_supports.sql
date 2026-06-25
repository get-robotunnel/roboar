-- migration 0002: human/principal agent identity + tunnel addressing contract
--
-- identity_kind distinguishes human/CLI principal agents from robot service agents.
-- tunnel_supports lists which channel types (control, meta, bulk…) the agent's
-- tunnel connection carries; set via heartbeat by the tunnel daemon.

ALTER TABLE agents ADD COLUMN identity_kind    TEXT     NOT NULL DEFAULT 'service';
ALTER TABLE agents ADD COLUMN tunnel_supports  TEXT[]   NOT NULL DEFAULT '{}';

-- 'service'   : robot / service-provider agent (existing default behavior)
-- 'principal' : human-side / CLI agent; consumer-only, defaults to private visibility

CREATE INDEX IF NOT EXISTS idx_agents_identity_kind ON agents(identity_kind);

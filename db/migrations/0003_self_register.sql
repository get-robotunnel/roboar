-- migration 0003: agent self-registration fields
--
-- Adds Ed25519 public_key (the agent's unique identity credential),
-- claim_status (unclaimed/claimed), and derived EVM wallet_address.
-- These three columns enable the self-register → heartbeat → claim flow
-- described in agent-registration-mcp-spec §1.

ALTER TABLE agents ADD COLUMN IF NOT EXISTS public_key     TEXT UNIQUE;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS claim_status   TEXT NOT NULL DEFAULT 'unclaimed';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS wallet_address TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_agents_public_key   ON agents(public_key) WHERE public_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agents_claim_status ON agents(claim_status);

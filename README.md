# Robot Agent Registry (RAR)

**Identity, discovery, and capability registry for robot agents** — so any agent on any machine can find and call any robot on the internet, without prior setup.

```python
from rar import Agent, Capability
import asyncio

agent = Agent(
    name="lidar-agent",
    capabilities=[
        Capability("get_scan", "Get a LiDAR point-cloud frame", permission="public"),
        Capability("stream_scan", "Stream continuous scans", permission="paid",
                   pricing={"model": "per_minute", "amount": 0.005, "token": "USDC"}),
    ]
)

@agent.capability("get_scan")
async def get_scan(params):
    return {"pcd_data": "<base64 encoded PCD>"}

asyncio.run(agent.start())
# ✓ keypair generated at ~/.rar/agent_key
# ✓ registered at reg.robotunnel.io — agent_id = agt_xxxxx
# ✓ wallet address derived from public key (receive x402 payments immediately)
# ✓ MCP server online at :11412
# ✓ discoverable by anyone — humans, AI agents, and other robots
```

---

## What RAR does (and doesn't do)

RAR stores **identity + discovery metadata + capability definitions + payment terms**. It deliberately does nothing else:

| What RAR does | What RAR doesn't do |
|---|---|
| Store who agents are (Ed25519 identity) | Relay traffic — that's [RoboTunnel](https://tunnel.robotunnel.io) |
| Store what they can do (capabilities) | Custody funds — that's x402 on-chain |
| Store how to reach them (tunnel endpoint) | Execute capabilities — that's the agent itself |
| Let anyone discover online agents | Require a server or account to get started |

RAR is infrastructure, not a product UI. Its primary interface is a REST API; its AI-agent interface is an MCP server.

---

## Three paths to registration

### 1. Robot agent (3 lines)

The `Agent` class handles everything: keypair generation, registration, MCP server, heartbeat.

```python
from rar import Agent, Capability
import asyncio

agent = Agent(name="my-robot", capabilities=[Capability("ping", "Returns pong")])

@agent.capability("ping")
async def ping(params): return {"result": "pong"}

asyncio.run(agent.start())
```

Your robot is now online and discoverable. It also has a deterministic EVM wallet address derived from its Ed25519 public key — ready to receive x402 payments before any human touches the machine.

### 2. Human / CLI identity (1 command)

```bash
pip install rar-agent
rar identity create --name "Russell's Laptop"
# Generates Ed25519 keypair, registers a principal identity
# → agent_id + owner_id, stored locally
```

Human-side agents (laptops, phones, CLI tools) register as `principal` identities — they have agent IDs and can be authorized to call robot capabilities, but don't appear in public discovery by default.

### 3. AI agent (MCP)

Add the registry MCP server to any MCP client (Claude, etc.):

```
https://reg.robotunnel.io/mcp
```

Available tools: `search_robot_agents`, `get_agent_details`, `get_connection_info`, `check_capability_price`, `register_identity`.

```
User: "Find an online robot with a LiDAR sensor"
Claude: [calls search_robot_agents(query="LiDAR", online_only=true)]
       → agt_B — lidar-agent — online — get_scan (public), stream_scan ($0.005/min)
```

---

## How agents get paid

When you mark a capability `permission="paid"`, the agent's MCP server automatically handles the x402 payment flow:

```
1. Caller invokes get_scan
2. MCP server → 402 { amount: 0.001, token: USDC, address: 0xABCD...  }
3. Caller pays on-chain, attaches proof (X-Payment header)
4. MCP server verifies → executes → returns result
```

The wallet address is deterministically derived from the agent's Ed25519 public key — so the robot can receive funds from the moment it registers, with no human involvement.

---

## Discovery

Public — no authentication required.

```bash
# Full-text search
curl "https://reg.robotunnel.io/v1/discover/agents?q=lidar&online=true"

# Get one agent's full details and connection info
curl "https://reg.robotunnel.io/v1/discover/agents/agt_xxx"
```

```json
{
  "agent_id": "agt_xxx",
  "name": "lidar-agent",
  "status": "online",
  "wallet_address": "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
  "connection": {
    "tunnel_endpoint": "rt://tunnel.robotunnel.io/agt_xxx",
    "mcp_endpoint": "https://tunnel.robotunnel.io/mcp/agt_xxx"
  },
  "capabilities": [
    { "name": "get_scan", "permission": "public" },
    { "name": "stream_scan", "permission": "paid", "pricing": { "amount": 0.005 } }
  ]
}
```

---

## Calling a remote agent

```python
from rar import AgentClient
import asyncio

async def main():
    client = AgentClient("https://tunnel.robotunnel.io/mcp/agt_xxx")
    result = await client.call("get_scan", {})
    print(result)  # {"pcd_data": "..."}

asyncio.run(main())
```

Robot-to-robot calls work the same way — no LLM needed. The tunnel handles NAT traversal transparently.

---

## Owner claim

By default, a self-registered agent is `unclaimed`: it runs, is discoverable, and can receive payments — but no human can withdraw the funds yet. Claim it to unlock fund management and policy control:

```bash
rar claim --agent-id agt_xxx
# Signs the agent_id with your local private key
# → you are now the verified owner
# → can set capability pricing, visibility, withdraw wallet balance
```

---

## Protocol boundaries

Three protocols, each at its own layer:

| Protocol | Layer | Who uses it | What it does |
|---|---|---|---|
| **REST** | Management | Agent → Registry | Register, heartbeat, config sync |
| **MCP** | Work interface | Caller → Agent | Invoke capabilities |
| **Tunnel** | Transport | Transparent | Move bytes from A to B |

Registration is always REST. MCP is the work interface, not the registration interface.

---

## Self-hosting

RAR needs only one binary and one Postgres database.

```bash
# 1. Database
createdb rar
export DATABASE_URL="postgres://localhost:5432/rar?sslmode=disable"
export JWT_SIGNING_KEY="$(openssl rand -hex 32)"

# 2. Run (migrations apply automatically on boot)
go run ./cmd/registry
# → listening on :8090

# 3. Verify
curl http://localhost:8090/v1/discover/agents
```

Point the SDK at your instance:

```python
agent = Agent(name="my-robot", registry_url="http://localhost:8090/v1", ...)
```

Or the CLI:

```bash
rar --registry http://localhost:8090/v1 identity create --name "My Laptop"
```

---

## Repository layout

```
cmd/registry/           Server entry point (Gin, :8090, auto-migrations)
internal/
  server/               Router + middleware
  auth/                 Ed25519 verify · JWT · bcrypt · Agent-Signature · EVM wallet
  store/                pgxpool data access
  model/                Entity types
  ids/                  Prefixed nanoid helpers (usr_/plt_/agt_/cap_)
db/migrations/          SQL schema (embedded, applied in order on boot)
sdk/python/rar/
  robot_agent.py        Agent class — self-register, MCP server, heartbeat
  agent_client.py       AgentClient — call remote agents
  agent.py              RARAgent — managed platform SDK (owner-provisioned)
  keys.py               Ed25519 keypair management (~/.rar/)
  cli.py                rar CLI (owner side)
  agent_cli.py          rar-agent CLI (platform side)
```

## API reference

Base URL: `https://reg.robotunnel.io/v1`

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/agents/self-register` | none | Self-register by Ed25519 public key (idempotent) |
| `POST` | `/agents/:id/heartbeat` | Agent-Signature | Update online status and connection endpoints |
| `POST` | `/agents/:id/claim` | Owner Ed25519 sig | Bind agent to human owner |
| `GET` | `/discover/agents` | none | Full-text search across public agents |
| `GET` | `/discover/agents/:id` | none | Get one agent's full details |
| `POST` | `/owners` | none | Register owner (Ed25519 public key) |
| `POST` | `/owners/auth/challenge` | none | Begin Ed25519 challenge-response login |
| `POST` | `/owners/auth/verify` | none | Verify challenge → JWT |
| `POST` | `/mcp` | none | Registry MCP server (AI agent entry point) |

## Status

**Live at `https://reg.robotunnel.io`**

| Feature | Status |
|---|---|
| Robot agent self-registration | ✅ |
| Agent-Signature authentication | ✅ |
| Deterministic EVM wallet address | ✅ |
| MCP server (registry discovery tools) | ✅ |
| Owner registration + Ed25519 login | ✅ |
| Public Discovery API | ✅ |
| Python SDK (`Agent`, `AgentClient`) | ✅ |
| x402 payment middleware (MCP) | ✅ basic |
| Owner claim flow | ✅ |
| Tunnel MCP proxy (`/mcp/:agent_id`) | ✅ direct |
| WebSocket config push | Phase 2 |
| On-chain identity anchoring | Phase 3 |
| Federation (multi-registry) | Phase 3 |

---

## License

Apache-2.0. See [LICENSE](./LICENSE).

Hosted at `reg.robotunnel.io` by [RoboTunnel](https://robotunnel.io). The protocol and this reference implementation are open; the hosted service and commercial operations layer are separate products.

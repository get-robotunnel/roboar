"""Robot Agent Registry — SDK.

Primary API (self-registering robot agents, spec §1-2):
    Agent        — self-registering robot agent with built-in MCP server
    Capability   — describes one capability exposed as an MCP tool
    AgentClient  — call remote robot agents' MCP tools (spec §4.2)

Platform API (existing, for managed platforms with human owners):
    RARAgent     — platform-side agent: register, capability CRUD, heartbeat
    RegistryClient — thin HTTP client over the registry API
"""

from .robot_agent import Agent, Capability
from .agent_client import AgentClient
from .agent import RARAgent
from .client import RegistryClient

__all__ = ["Agent", "Capability", "AgentClient", "RARAgent", "RegistryClient"]
__version__ = "0.2.0"

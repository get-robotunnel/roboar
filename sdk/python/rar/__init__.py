"""Robot Agent Registry — platform-side SDK.

Public surface:
    RARAgent, Capability  — register agents/capabilities and heartbeat (spec §4).
    RegistryClient        — thin HTTP client over the registry API.
"""

from .agent import RARAgent, Capability
from .client import RegistryClient

__all__ = ["RARAgent", "Capability", "RegistryClient"]
__version__ = "0.1.0"

"""Platform-side agent SDK (spec §4): register agents/capabilities and heartbeat."""

import threading
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional

from . import keys
from .client import DEFAULT_BASE_URL, RegistryClient


@dataclass
class Capability:
    name: str
    display_name: str
    interface_type: str
    description: str = ""
    permission: str = "public"
    pricing: Optional[dict] = None
    input_schema: Optional[dict] = None
    output_schema: Optional[dict] = None
    ros2: Optional[dict] = None
    enabled: bool = True

    def to_body(self) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "name": self.name,
            "display_name": self.display_name,
            "interface_type": self.interface_type,
            "description": self.description,
            "permission": self.permission,
            "enabled": self.enabled,
        }
        if self.pricing is not None:
            body["pricing"] = self.pricing
        if self.input_schema is not None:
            body["input_schema"] = self.input_schema
        if self.output_schema is not None:
            body["output_schema"] = self.output_schema
        if self.ros2 is not None:
            body["ros2"] = self.ros2
        return body


@dataclass
class AgentHandle:
    agent_id: str
    _client: RegistryClient
    _auth: str

    def register_capability(self, cap: Capability) -> dict:
        return self._client.expect(
            "POST", f"/agents/{self.agent_id}/capabilities", cap.to_body(), self._auth
        )


class RARAgent:
    """Connects a platform to the registry: registers agents, then heartbeats."""

    def __init__(
        self,
        platform_token: str,
        platform_id: str,
        registry_url: str = DEFAULT_BASE_URL,
        platform_key_path: Path = keys.PLATFORM_KEY,
    ):
        if not platform_token or not platform_id:
            raise ValueError("platform_token and platform_id are required")
        self.platform_token = platform_token
        self.platform_id = platform_id
        self.client = RegistryClient(registry_url)
        self.priv = keys.load_or_create(platform_key_path)
        self._config_cb: Optional[Callable[[dict], None]] = None
        self._stop = threading.Event()

    @property
    def _auth(self) -> str:
        return "Platform " + self.platform_token

    def register_agent(
        self,
        name: str,
        description: str,
        agent_type: str,
        version: str,
        visibility: str = "public",
        connection: Optional[dict] = None,
        metadata: Optional[dict] = None,
    ) -> AgentHandle:
        body = {
            "name": name,
            "description": description,
            "agent_type": agent_type,
            "version": version,
            "visibility": visibility,
            "connection": connection or {},
        }
        if metadata is not None:
            body["metadata"] = metadata
        payload = self.client.expect(
            "POST", f"/platforms/{self.platform_id}/agents", body, self._auth
        )
        return AgentHandle(agent_id=payload["agent_id"], _client=self.client, _auth=self._auth)

    def on_config_update(self, cb: Callable[[dict], None]) -> None:
        """Register a config-change callback. Wiring is Phase 2 (WebSocket push)."""
        self._config_cb = cb

    def heartbeat_once(self) -> None:
        self.client.expect(
            "POST",
            f"/platforms/{self.platform_id}/heartbeat",
            {"public_key": keys.public_key_hex(self.priv), "status": "online"},
            self._auth,
        )

    def start(self, interval: int = 30) -> None:
        """Blocking heartbeat loop until stop() is called or interrupted."""
        self._stop.clear()
        while not self._stop.is_set():
            try:
                self.heartbeat_once()
            except Exception as exc:  # keep the loop alive across transient errors
                print(f"[rar-agent] heartbeat failed: {exc}")
            self._stop.wait(interval)

    def stop(self) -> None:
        self._stop.set()


def operations_preset() -> List[Capability]:
    """Baseline capabilities for `rar-agent start --preset operations` (spec §5)."""
    return [
        Capability(
            name="get_system_status",
            display_name="Get System Status",
            interface_type="mcp_tool",
            description="Returns CPU, memory, and robot operational status",
            permission="public",
            input_schema={"type": "object", "properties": {}},
            output_schema={
                "type": "object",
                "properties": {
                    "cpu_percent": {"type": "number"},
                    "memory_percent": {"type": "number"},
                    "uptime_seconds": {"type": "number"},
                },
            },
        ),
        Capability(
            name="execute_command",
            display_name="Execute Command",
            interface_type="mcp_tool",
            description="Run a shell command on the robot host",
            permission="owner_only",
        ),
        Capability(
            name="reboot",
            display_name="Reboot Robot",
            interface_type="mcp_tool",
            description="Reboot the robot host",
            permission="owner_only",
        ),
    ]

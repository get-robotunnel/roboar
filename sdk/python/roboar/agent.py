"""Platform-managed agent SDK: register agents/capabilities and heartbeat."""

import threading
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional

from . import keys
from .client import DEFAULT_BASE_URL, RegistryClient
from .robot_agent import Capability


class AgentHandle:
    def __init__(self, agent_id: str, _client: RegistryClient, _auth: str) -> None:
        self.agent_id = agent_id
        self._client = _client
        self._auth = _auth

    def register_capability(self, cap: Capability) -> dict:
        return self._client.expect(
            "POST", f"/agents/{self.agent_id}/capabilities", cap.to_register_body(), self._auth
        )


class PlatformAgent:
    """Connects a platform to the registry: registers agents, then heartbeats.

    For use when a human operator owns the platform and provisions agents
    on behalf of robots (as opposed to self-registering agents using Agent).
    """

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

    def heartbeat_once(
        self,
        agent_tunnels: Optional[List[Dict[str, Any]]] = None,
    ) -> None:
        body: Dict[str, Any] = {
            "public_key": keys.public_key_hex(self.priv),
            "status": "online",
        }
        if agent_tunnels:
            body["agents"] = agent_tunnels
        self.client.expect(
            "POST",
            f"/platforms/{self.platform_id}/heartbeat",
            body,
            self._auth,
        )

    def start(
        self,
        interval: int = 30,
        agent_tunnels: Optional[List[Dict[str, Any]]] = None,
    ) -> None:
        """Blocking heartbeat loop until stop() is called or interrupted."""
        self._stop.clear()
        while not self._stop.is_set():
            try:
                self.heartbeat_once(agent_tunnels=agent_tunnels)
            except Exception as exc:
                print(f"[roboar] heartbeat failed: {exc}")
            self._stop.wait(interval)

    def stop(self) -> None:
        self._stop.set()


def operations_preset() -> List[Capability]:
    """Baseline capabilities for `roboar agent start --preset operations`."""
    return [
        Capability(
            name="get_system_status",
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
            description="Run a shell command on the robot host",
            permission="owner_only",
        ),
        Capability(
            name="reboot",
            description="Reboot the robot host",
            permission="owner_only",
        ),
    ]

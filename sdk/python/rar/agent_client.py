"""AgentClient: call a remote robot agent's MCP tools (spec §4.2)."""
from __future__ import annotations

import asyncio
import json
import urllib.error
import urllib.request
from typing import Any, Dict, Optional


class AgentClientError(Exception):
    def __init__(self, status: int, payload: Any) -> None:
        self.status = status
        self.payload = payload
        msg = payload.get("error") if isinstance(payload, dict) else str(payload)
        super().__init__(f"agent error {status}: {msg}")


class AgentClient:
    """Connects to a robot agent's MCP server and calls its tools.

    Works with any MCP endpoint: direct local address, tunnel-proxied URL, or
    any URL returned by the registry's mcp_endpoint field.

    Example (robot-to-robot, spec §4.2):
        client = AgentClient("https://tunnel.robotunnel.io/mcp/agt_xxx")
        result = await client.call("get_scan", {})
    """

    def __init__(self, mcp_url: str) -> None:
        self.mcp_url = mcp_url.rstrip("/")
        self._call_id = 0

    async def call(self, tool_name: str, params: Optional[Dict[str, Any]] = None) -> Any:
        """Invoke MCP tool *tool_name* with *params* and return the result dict."""
        self._call_id += 1
        body = json.dumps({
            "jsonrpc": "2.0",
            "id": self._call_id,
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": params or {},
            },
        }).encode()

        loop = asyncio.get_event_loop()
        status, payload = await loop.run_in_executor(None, self._post, body, {})

        if status == 402:
            # x402 payment required — stub: raise with payment terms so caller can act.
            raise AgentClientError(402, payload)

        if status not in (200, 201):
            raise AgentClientError(status, payload)

        if isinstance(payload, dict) and "error" in payload:
            err = payload["error"]
            raise AgentClientError(-1, err)

        result = (payload or {}).get("result", {})
        # Unwrap MCP content block if present.
        content = result.get("content") if isinstance(result, dict) else None
        if content and isinstance(content, list) and content[0].get("type") == "text":
            text = content[0].get("text", "")
            try:
                return json.loads(text)
            except json.JSONDecodeError:
                return {"text": text}
        return result

    async def initialize(self) -> Dict[str, Any]:
        """Send MCP initialize handshake and return server info."""
        self._call_id += 1
        body = json.dumps({
            "jsonrpc": "2.0",
            "id": self._call_id,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "rar-agent-client", "version": "0.1"},
            },
        }).encode()
        loop = asyncio.get_event_loop()
        _, payload = await loop.run_in_executor(None, self._post, body, {})
        return (payload or {}).get("result", {})

    async def list_tools(self) -> list:
        """Return the agent's tool list (its capabilities as MCP tools)."""
        self._call_id += 1
        body = json.dumps({
            "jsonrpc": "2.0",
            "id": self._call_id,
            "method": "tools/list",
            "params": {},
        }).encode()
        loop = asyncio.get_event_loop()
        _, payload = await loop.run_in_executor(None, self._post, body, {})
        result = (payload or {}).get("result", {})
        return result.get("tools", [])

    def _post(self, body: bytes, extra_headers: Dict[str, str]) -> tuple[int, Any]:
        req = urllib.request.Request(self.mcp_url, data=body, method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Content-Length", str(len(body)))
        for k, v in extra_headers.items():
            req.add_header(k, v)
        try:
            with urllib.request.urlopen(req) as resp:
                raw = resp.read()
                return resp.status, (json.loads(raw) if raw else None)
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            return exc.code, (json.loads(raw) if raw else None)

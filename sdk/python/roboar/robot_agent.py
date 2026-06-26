"""Self-registering robot agent SDK (spec §1-2): Agent and Capability.

Usage:
    from roboar import Agent, Capability

    agent = Agent(
        name="lidar-agent",
        capabilities=[Capability("get_scan", "Get a LiDAR scan", permission="public")]
    )

    @agent.capability("get_scan")
    async def get_scan(params):
        return {"pcd_data": "..."}

    asyncio.run(agent.start())
"""
from __future__ import annotations

import asyncio
import hashlib
import json
import os
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional

from . import keys
from .client import DEFAULT_BASE_URL, RegistryClient

AGENT_KEY = keys.KEY_DIR / "agent_key"
DEFAULT_MCP_PORT = 11412


@dataclass
class Capability:
    """A single capability exposed by an Agent as an MCP tool."""

    name: str
    description: str = ""
    permission: str = "public"  # public | authenticated | paid | owner_only
    pricing: Optional[dict] = None
    interface_type: str = "mcp_tool"
    display_name: Optional[str] = None
    input_schema: Optional[dict] = None
    output_schema: Optional[dict] = None
    ros2: Optional[dict] = None
    enabled: bool = True

    def __post_init__(self) -> None:
        if self.display_name is None:
            self.display_name = self.name.replace("_", " ").title()

    def to_register_body(self) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "name": self.name,
            "display_name": self.display_name,
            "description": self.description,
            "interface_type": self.interface_type,
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

    def to_mcp_tool(self) -> Dict[str, Any]:
        desc = self.description
        if self.pricing:
            amt = self.pricing.get("amount", "")
            token = self.pricing.get("token", "USDC")
            model_name = self.pricing.get("model", "per_call")
            desc = f"{desc} Cost: {amt} {token} {model_name}."
        return {
            "name": self.name,
            "description": desc.strip(),
            "inputSchema": self.input_schema or {"type": "object", "properties": {}},
        }


class Agent:
    """A self-registering robot agent (spec §1.2).

    On ``start()`` the agent:
    1. Loads or generates an Ed25519 keypair at ``key_path`` (default ~/.roboar/agent_key).
    2. Self-registers with the registry (idempotent — safe to restart).
    3. Starts an MCP server on ``mcp_port`` (default 11412).
    4. Loops heartbeats every ``heartbeat_interval`` seconds.
    """

    def __init__(
        self,
        name: str,
        description: str = "",
        capabilities: Optional[List[Capability]] = None,
        agent_type: str = "robot",
        version: str = "1.0",
        registry_url: str = DEFAULT_BASE_URL,
        key_path: Optional[Path] = None,
        mcp_port: int = DEFAULT_MCP_PORT,
        mcp_host: str = "0.0.0.0",
        heartbeat_interval: int = 30,
    ) -> None:
        self.name = name
        self.description = description
        self.capabilities: List[Capability] = capabilities or []
        self.agent_type = agent_type
        self.version = version
        self.registry_url = registry_url
        self.key_path = key_path or AGENT_KEY
        self.mcp_port = mcp_port
        self.mcp_host = mcp_host
        self.heartbeat_interval = heartbeat_interval

        self._handlers: Dict[str, Callable] = {}
        self._client = RegistryClient(registry_url)

        # Set after start():
        self.agent_id: Optional[str] = None
        self.wallet_address: Optional[str] = None
        self.priv = None

    # ── decorator ──────────────────────────────────────────────────────────

    def capability(self, name: str) -> Callable:
        """Bind a coroutine as the implementation for capability *name*.

        The coroutine receives ``params: dict`` and must return a ``dict``.
        """
        def decorator(fn: Callable) -> Callable:
            self._handlers[name] = fn
            return fn
        return decorator

    # ── lifecycle ──────────────────────────────────────────────────────────

    async def start(self) -> None:
        """Complete steps 1-6 from spec §1.2, then block until interrupted."""
        self.priv = keys.load_or_create(self.key_path)
        pub_hex = keys.public_key_hex(self.priv)
        await asyncio.get_event_loop().run_in_executor(None, self._register, pub_hex)
        print(f"[roboar] {self.name} online  agent_id={self.agent_id}  wallet={self.wallet_address}")
        await asyncio.gather(
            self._serve_mcp(),
            self._heartbeat_loop(),
        )

    def _register(self, pub_hex: str) -> None:
        body = {
            "public_key": pub_hex,
            "name": self.name,
            "description": self.description,
            "agent_type": self.agent_type,
            "version": self.version,
            "capabilities": [c.to_register_body() for c in self.capabilities],
        }
        resp = self._client.expect("POST", "/agents", body)
        self.agent_id = resp["agent_id"]
        self.wallet_address = resp.get("wallet_address", "")

    # ── heartbeat ──────────────────────────────────────────────────────────

    async def _heartbeat_loop(self) -> None:
        while True:
            try:
                await asyncio.get_event_loop().run_in_executor(None, self._send_heartbeat)
            except Exception as exc:
                print(f"[roboar] heartbeat failed: {exc}")
            await asyncio.sleep(self.heartbeat_interval)

    def _send_heartbeat(self) -> None:
        mcp_ep = f"http://127.0.0.1:{self.mcp_port}"
        body: Dict[str, Any] = {
            "status": "online",
            "tunnel_endpoint": "",
            "mcp_endpoint": mcp_ep,
        }
        body_bytes = json.dumps(body).encode()
        headers = self._agent_sig_headers(body_bytes)
        req = urllib.request.Request(
            self._client.base + f"/agents/{self.agent_id}/heartbeat",
            data=body_bytes,
            method="POST",
        )
        req.add_header("Content-Type", "application/json")
        for k, v in headers.items():
            req.add_header(k, v)
        try:
            with urllib.request.urlopen(req) as _:
                pass
        except urllib.error.HTTPError as exc:
            raise RuntimeError(f"heartbeat HTTP {exc.code}") from exc

    def _agent_sig_headers(self, body_bytes: bytes) -> Dict[str, str]:
        nonce = os.urandom(32).hex()
        ts = str(int(time.time()))
        body_hash = hashlib.sha256(body_bytes).hexdigest()
        message = (self.agent_id + nonce + body_hash).encode()
        sig = self.priv.sign(message).hex()
        return {
            "X-Agent-ID": self.agent_id,
            "X-Agent-Nonce": nonce,
            "X-Agent-Signature": sig,
            "X-Agent-Timestamp": ts,
        }

    # ── MCP server ─────────────────────────────────────────────────────────

    async def _serve_mcp(self) -> None:
        server = await asyncio.start_server(
            self._handle_http, self.mcp_host, self.mcp_port
        )
        addr = server.sockets[0].getsockname()
        print(f"[roboar] MCP server listening on {addr[0]}:{addr[1]}")
        async with server:
            await server.serve_forever()

    async def _handle_http(
        self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter
    ) -> None:
        try:
            line = await reader.readline()
            if not line:
                return
            parts = line.decode(errors="replace").split()
            if len(parts) < 2:
                return
            method = parts[0].upper()

            hdrs: Dict[str, str] = {}
            while True:
                hline = await reader.readline()
                if hline in (b"\r\n", b"\n", b""):
                    break
                decoded = hline.decode(errors="replace").strip()
                if ":" in decoded:
                    k, v = decoded.split(":", 1)
                    hdrs[k.strip().lower()] = v.strip()

            content_length = int(hdrs.get("content-length", 0))
            body = await reader.readexactly(content_length) if content_length > 0 else b""

            if method == "OPTIONS":
                await self._write_response(writer, 200, b"{}")
                return

            if method != "POST":
                await self._write_response(writer, 405, json.dumps({"error": "method not allowed"}).encode())
                return

            status, resp_body = await self._dispatch_jsonrpc(body, hdrs)
            await self._write_response(writer, status, resp_body)
        except (asyncio.IncompleteReadError, ConnectionResetError):
            pass
        finally:
            try:
                writer.close()
                await writer.wait_closed()
            except Exception:
                pass

    async def _dispatch_jsonrpc(
        self, body: bytes, hdrs: Dict[str, str]
    ) -> tuple[int, bytes]:
        try:
            req = json.loads(body or b"{}")
        except json.JSONDecodeError:
            return 400, _rpc_error(None, -32700, "parse error")

        rpc_id = req.get("id")
        method = req.get("method", "")

        if rpc_id is None and method:
            return 202, b""

        if method == "initialize":
            result = {
                "protocolVersion": "2024-11-05",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": self.name, "version": self.version},
            }
            return 200, _rpc_ok(rpc_id, result)

        if method == "tools/list":
            return 200, _rpc_ok(rpc_id, {"tools": [c.to_mcp_tool() for c in self.capabilities]})

        if method == "tools/call":
            params = req.get("params", {})
            tool_name = params.get("name", "")
            arguments = params.get("arguments") or {}
            return await self._call_tool(rpc_id, tool_name, arguments, hdrs)

        return 200, _rpc_error(rpc_id, -32601, f"method not found: {method}")

    async def _call_tool(
        self, rpc_id: Any, tool_name: str, arguments: dict, hdrs: Dict[str, str]
    ) -> tuple[int, bytes]:
        cap = next((c for c in self.capabilities if c.name == tool_name), None)
        if cap is None:
            return 200, _rpc_error(rpc_id, -32601, f"unknown tool: {tool_name}")

        perm_result = await self._check_permission(cap, hdrs)
        if perm_result is not None:
            return perm_result

        handler = self._handlers.get(tool_name)
        if handler is None:
            return 200, _rpc_error(rpc_id, -32603, f"no handler registered for {tool_name}")

        try:
            if asyncio.iscoroutinefunction(handler):
                result = await handler(arguments)
            else:
                result = await asyncio.get_event_loop().run_in_executor(None, handler, arguments)
        except Exception as exc:
            return 200, _rpc_error(rpc_id, -32603, str(exc))

        return 200, _rpc_ok(rpc_id, {
            "content": [{"type": "text", "text": json.dumps(result)}],
            "isError": False,
        })

    async def _check_permission(
        self, cap: Capability, hdrs: Dict[str, str]
    ) -> Optional[tuple[int, bytes]]:
        if cap.permission == "public":
            return None

        if cap.permission == "paid":
            payment = hdrs.get("x-payment", "")
            if not payment:
                terms = {
                    "x402": {
                        "amount": (cap.pricing or {}).get("amount", 0),
                        "token": (cap.pricing or {}).get("token", "USDC"),
                        "address": self.wallet_address or "",
                        "network": (cap.pricing or {}).get("network", "base"),
                        "description": f"{cap.name}",
                    }
                }
                body = json.dumps(terms).encode()
                return 402, body

        if cap.permission in ("authenticated", "owner_only"):
            # TODO Phase 2: verify caller Agent-Signature or owner JWT
            pass

        return None

    @staticmethod
    async def _write_response(
        writer: asyncio.StreamWriter, status: int, body: bytes
    ) -> None:
        status_text = {200: "OK", 202: "Accepted", 400: "Bad Request",
                       402: "Payment Required", 405: "Method Not Allowed"}.get(status, "OK")
        response = (
            f"HTTP/1.1 {status} {status_text}\r\n"
            f"Content-Type: application/json\r\n"
            f"Content-Length: {len(body)}\r\n"
            f"Access-Control-Allow-Origin: *\r\n"
            f"\r\n"
        ).encode() + body
        writer.write(response)
        await writer.drain()


# ── JSON-RPC helpers ───────────────────────────────────────────────────────────

def _rpc_ok(rpc_id: Any, result: Any) -> bytes:
    return json.dumps({"jsonrpc": "2.0", "id": rpc_id, "result": result}).encode()


def _rpc_error(rpc_id: Any, code: int, message: str) -> bytes:
    return json.dumps({
        "jsonrpc": "2.0",
        "id": rpc_id,
        "error": {"code": code, "message": message},
    }).encode()

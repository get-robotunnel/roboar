"""Minimal HTTP client over the registry API (stdlib only)."""

import json
import urllib.error
import urllib.request
from typing import Any, Optional, Tuple

DEFAULT_BASE_URL = "https://reg.robotunnel.io/v1"


class RegistryError(Exception):
    def __init__(self, status: int, payload: Any):
        self.status = status
        self.payload = payload
        msg = payload.get("error") if isinstance(payload, dict) else payload
        super().__init__(f"registry error {status}: {msg}")


class RegistryClient:
    def __init__(self, base_url: str = DEFAULT_BASE_URL):
        self.base = base_url.rstrip("/")

    def request(
        self,
        method: str,
        path: str,
        body: Optional[dict] = None,
        auth: Optional[str] = None,
    ) -> Tuple[int, Any]:
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if auth:
            req.add_header("Authorization", auth)
        try:
            with urllib.request.urlopen(req) as resp:
                raw = resp.read()
                return resp.status, (json.loads(raw) if raw else None)
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            return exc.code, (json.loads(raw) if raw else None)

    def expect(self, method: str, path: str, body=None, auth=None, ok=(200, 201, 204)) -> Any:
        status, payload = self.request(method, path, body, auth)
        if status not in ok:
            raise RegistryError(status, payload)
        return payload

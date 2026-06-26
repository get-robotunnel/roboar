"""Owner CLI session persistence (~/.roboar/session.json)."""

import json
import os

from .keys import KEY_DIR

SESSION = KEY_DIR / "session.json"


def load() -> dict:
    if SESSION.exists():
        try:
            return json.loads(SESSION.read_text())
        except ValueError:
            return {}
    return {}


def save(data: dict) -> None:
    KEY_DIR.mkdir(parents=True, exist_ok=True)
    SESSION.write_text(json.dumps(data, indent=2))
    try:
        os.chmod(SESSION, 0o600)
    except OSError:
        pass

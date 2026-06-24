"""Local Ed25519 key handling for owners and platforms.

Keys are stored as raw 32-byte seeds under ~/.rar/ with 0600 permissions.
"""

import os
from pathlib import Path

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

KEY_DIR = Path(os.path.expanduser("~/.rar"))
_RAW = serialization.Encoding.Raw
_RAW_PRIV = serialization.PrivateFormat.Raw
_RAW_PUB = serialization.PublicFormat.Raw


def _ensure_dir() -> None:
    KEY_DIR.mkdir(parents=True, exist_ok=True)
    try:
        os.chmod(KEY_DIR, 0o700)
    except OSError:
        pass


def generate_keypair(path: Path) -> Ed25519PrivateKey:
    """Generate a keypair, persist the private seed at *path*, and return it."""
    _ensure_dir()
    priv = Ed25519PrivateKey.generate()
    seed = priv.private_bytes(_RAW, _RAW_PRIV, serialization.NoEncryption())
    path.write_bytes(seed)
    os.chmod(path, 0o600)
    return priv


def load_or_create(path: Path) -> Ed25519PrivateKey:
    if path.exists():
        return Ed25519PrivateKey.from_private_bytes(path.read_bytes())
    return generate_keypair(path)


def load_private_key(path: Path) -> Ed25519PrivateKey:
    return Ed25519PrivateKey.from_private_bytes(path.read_bytes())


def public_key_hex(priv: Ed25519PrivateKey) -> str:
    return priv.public_key().public_bytes(_RAW, _RAW_PUB).hex()


def sign_hex(priv: Ed25519PrivateKey, message: bytes) -> str:
    """Sign *message* and return the signature as hex (matches the server's
    VerifyEd25519, which verifies over the raw challenge string bytes)."""
    return priv.sign(message).hex()


# Default key locations.
OWNER_KEY = KEY_DIR / "key"
PLATFORM_KEY = KEY_DIR / "platform_key"

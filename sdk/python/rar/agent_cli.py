"""`rar-agent` — robot-side entry point: register an agent and heartbeat."""

import argparse
import os
import sys

from .agent import RARAgent, operations_preset
from .client import DEFAULT_BASE_URL, RegistryError


def cmd_start(args):
    token = args.platform_token or os.environ.get("RAR_PLATFORM_TOKEN")
    platform_id = args.platform_id or os.environ.get("RAR_PLATFORM_ID")
    registry_url = args.registry_url or os.environ.get("RAR_REGISTRY_URL") or DEFAULT_BASE_URL
    if not token or not platform_id:
        sys.exit("Set RAR_PLATFORM_TOKEN and RAR_PLATFORM_ID (or pass --platform-token/--platform-id).")

    rar = RARAgent(token, platform_id, registry_url=registry_url)

    if args.preset == "operations":
        handle = rar.register_agent(
            name=args.name,
            description="Manages and controls this robot",
            agent_type="operations",
            version="1.0.0",
            visibility="public",
        )
        caps = operations_preset()
    else:
        sys.exit(f"Unknown preset: {args.preset}")

    for cap in caps:
        handle.register_capability(cap)

    print(f"agent_id = {handle.agent_id}")
    print(f"Registered {len(caps)} capabilities: " + ", ".join(f"{c.name} ({c.permission})" for c in caps))
    print(f"Heartbeat every {args.interval}s. Registry: {registry_url}")
    print("Press Ctrl-C to stop.")
    try:
        rar.start(interval=args.interval)
    except KeyboardInterrupt:
        rar.stop()
        print("\nStopped.")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="rar-agent", description="Robot Agent Registry agent runtime")
    sub = p.add_subparsers(dest="cmd", required=True)
    start = sub.add_parser("start", help="register and start heartbeating")
    start.add_argument("--preset", default="operations", help="agent preset (default: operations)")
    start.add_argument("--name", default="operations-agent")
    start.add_argument("--platform-token")
    start.add_argument("--platform-id")
    start.add_argument("--registry-url")
    start.add_argument("--interval", type=int, default=30)
    start.set_defaults(func=cmd_start)
    return p


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        args.func(args)
    except RegistryError as exc:
        sys.exit(str(exc))


if __name__ == "__main__":
    main()

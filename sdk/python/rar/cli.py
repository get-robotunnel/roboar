"""`rar` — owner-side CLI: register, login, manage platforms, discover agents."""

import argparse
import os
import sys

from . import keys, session
from .client import DEFAULT_BASE_URL, RegistryClient, RegistryError


def resolve_registry_url(args) -> str:
    return (
        getattr(args, "registry_url", None)
        or os.environ.get("RAR_REGISTRY_URL")
        or session.load().get("registry_url")
        or DEFAULT_BASE_URL
    )


def cmd_auth_register(args):
    client = RegistryClient(resolve_registry_url(args))
    priv = keys.generate_keypair(keys.OWNER_KEY)
    pub = keys.public_key_hex(priv)
    body = {"public_key": pub, "display_name": args.name}
    if args.email:
        body["email"] = args.email
    owner = client.expect("POST", "/owners", body)
    sess = session.load()
    sess.update({"registry_url": client.base, "owner_id": owner["owner_id"], "public_key": pub})
    session.save(sess)
    print(f"Registered owner {owner['owner_id']}")
    print(f"Keypair stored at {keys.OWNER_KEY}")


def cmd_auth_login(args):
    client = RegistryClient(resolve_registry_url(args))
    priv = keys.load_private_key(keys.OWNER_KEY)
    pub = keys.public_key_hex(priv)
    ch = client.expect("POST", "/owners/auth/challenge", {"public_key": pub})
    challenge = ch["challenge"]
    signature = keys.sign_hex(priv, challenge.encode())
    res = client.expect(
        "POST", "/owners/auth/verify",
        {"public_key": pub, "challenge": challenge, "signature": signature},
    )
    sess = session.load()
    sess.update({"registry_url": client.base, "owner_id": res["owner_id"], "token": res["token"]})
    session.save(sess)
    print(f"Logged in as {res['owner_id']} (token valid ~24h)")


def cmd_platform_register(args):
    sess = session.load()
    token = sess.get("token")
    if not token:
        sys.exit("Not logged in. Run `rar auth login` first.")
    client = RegistryClient(resolve_registry_url(args))
    body = {"platform_type": args.type, "display_name": args.name}
    if args.description:
        body["description"] = args.description
    if args.tags:
        body["tags"] = [t.strip() for t in args.tags.split(",") if t.strip()]
    res = client.expect("POST", "/platforms", body, auth="Bearer " + token)
    platform = res["platform"]
    print(f"platform_id    = {platform['platform_id']}")
    print(f"platform_token = {res['platform_token']}   <-- copy to the robot (shown once)")
    print()
    print("On the robot, set:")
    print(f"  export RAR_PLATFORM_TOKEN={res['platform_token']}")
    print(f"  export RAR_PLATFORM_ID={platform['platform_id']}")
    print(f"  export RAR_REGISTRY_URL={client.base}")


def cmd_identity_create(args):
    client = RegistryClient(resolve_registry_url(args))
    priv = keys.load_or_create(keys.OWNER_KEY)
    pub = keys.public_key_hex(priv)
    body = {"public_key": pub, "display_name": args.name, "kind": "principal"}
    res = client.expect("POST", "/identities/quick", body)
    sess = session.load()
    sess.update({
        "registry_url": client.base,
        "owner_id": res["owner_id"],
        "agent_id": res["agent_id"],
        "platform_id": res["platform_id"],
        "platform_token": res["platform_token"],
        "public_key": pub,
    })
    session.save(sess)
    print(f"Identity created:")
    print(f"  agent_id       = {res['agent_id']}")
    print(f"  owner_id       = {res['owner_id']}")
    print(f"  platform_id    = {res['platform_id']}")
    print(f"  platform_token = {res['platform_token']}   <-- saved locally, shown once")
    print(f"  keypair stored at {keys.OWNER_KEY}")


def cmd_discover_agents(args):
    client = RegistryClient(resolve_registry_url(args))
    query = []
    if args.online:
        query.append("online=true")
    if args.platform_type:
        query.append("platform_type=" + args.platform_type)
    if args.q:
        query.append("q=" + args.q)
    if args.tags:
        query.append("tags=" + args.tags)
    path = "/discover/agents" + ("?" + "&".join(query) if query else "")
    res = client.expect("GET", path)
    agents = res.get("agents") or []
    if not agents:
        print("No agents found.")
        return
    print(f"{res.get('total', len(agents))} agent(s):")
    for a in agents:
        online = "online" if a.get("online") else "offline"
        caps = ", ".join(c["name"] for c in a.get("capabilities") or [])
        print(f"  {a['agent_id']}  {a['name']}  [{a['platform_type']}]  {online}")
        print(f"      owner={a.get('owner_display_name', '?')}  capabilities=[{caps}]")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="rar", description="Robot Agent Registry CLI")
    sub = p.add_subparsers(dest="group", required=True)

    auth = sub.add_parser("auth", help="account registration and login").add_subparsers(dest="cmd", required=True)
    reg = auth.add_parser("register", help="generate a keypair and register an owner")
    reg.add_argument("--name", required=True)
    reg.add_argument("--email")
    reg.add_argument("--registry-url")
    reg.set_defaults(func=cmd_auth_register)
    login = auth.add_parser("login", help="sign a challenge to obtain a session token")
    login.add_argument("--registry-url")
    login.set_defaults(func=cmd_auth_login)

    platform = sub.add_parser("platform", help="manage platforms").add_subparsers(dest="cmd", required=True)
    preg = platform.add_parser("register", help="register a platform and mint its token")
    preg.add_argument("--name", required=True)
    preg.add_argument("--type", required=True,
                      help="raspberry_pi|ros2_robot|mac|mobile|cloud|other")
    preg.add_argument("--tags", help="comma-separated tags")
    preg.add_argument("--description")
    preg.add_argument("--registry-url")
    preg.set_defaults(func=cmd_platform_register)

    identity = sub.add_parser("identity", help="manage your identity").add_subparsers(dest="cmd", required=True)
    icreate = identity.add_parser("create", help="register a principal identity (one step)")
    icreate.add_argument("--name", required=True, help="display name for this identity, e.g. \"Russell's Laptop\"")
    icreate.add_argument("--registry-url")
    icreate.set_defaults(func=cmd_identity_create)

    discover = sub.add_parser("discover", help="discover public agents").add_subparsers(dest="cmd", required=True)
    dag = discover.add_parser("agents", help="search public agents")
    dag.add_argument("--online", action="store_true")
    dag.add_argument("--platform-type")
    dag.add_argument("--q")
    dag.add_argument("--tags")
    dag.add_argument("--registry-url")
    dag.set_defaults(func=cmd_discover_agents)

    return p


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        args.func(args)
    except RegistryError as exc:
        sys.exit(str(exc))
    except FileNotFoundError:
        sys.exit("No local key found. Run `rar auth register` first.")


if __name__ == "__main__":
    main()

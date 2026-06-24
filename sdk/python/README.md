# rar-agent (Python SDK + CLI)

Platform-side SDK and CLIs for the Robot Agent Registry.

## Install

```bash
pip install ./sdk/python      # or: pip install rar-agent (once published)
```

Installs two commands: `rar` (owner-side) and `rar-agent` (robot-side).

## Owner workflow (laptop)

```bash
export RAR_REGISTRY_URL=https://reg.robotunnel.io/v1   # optional; this is the default

rar auth register --name "Russell" --email russell@example.com   # generates ~/.rar/key, registers
rar auth login                                                   # Ed25519 challenge → session token

rar platform register --name "Russell's RPi 4" --type raspberry_pi --tags "lidar,outdoor"
# → prints platform_id + platform_token (token shown once)

rar discover agents --online --platform-type raspberry_pi
```

## Robot workflow (Raspberry Pi / ROS2 host)

```bash
export RAR_PLATFORM_TOKEN=ptk_...    # from `rar platform register`
export RAR_PLATFORM_ID=plt_...
export RAR_REGISTRY_URL=https://reg.robotunnel.io/v1

rar-agent start --preset operations
# Registers an operations agent + baseline capabilities, then heartbeats every 30s.
```

## Library usage

```python
from rar import RARAgent, Capability

rar = RARAgent(platform_token=..., platform_id=..., registry_url="https://reg.robotunnel.io/v1")
agent = rar.register_agent(name="lidar-agent", description="...", agent_type="sensor", version="1.0.0")
agent.register_capability(Capability(
    name="get_lidar_scan", display_name="Get LiDAR Point Cloud", interface_type="ros2_topic",
    permission="paid", pricing={"model": "per_call", "amount": 0.001},
    ros2={"node_name": "lidar_node", "topic_name": "/lidar/points", "message_type": "sensor_msgs/PointCloud2"},
))
rar.start()   # blocking heartbeat loop
```

Requires Python 3.9+ and `cryptography`.

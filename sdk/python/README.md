# roboar (Python SDK + CLI)

SDK and CLI for the Robot Agent Registry.

## Install

```bash
pip install ./sdk/python      # or: pip install roboar (once published)
```

Installs one command: `roboar`.

## Owner workflow (laptop)

```bash
export ROBOAR_REGISTRY_URL=https://reg.robotunnel.io/v1   # optional; this is the default

roboar auth register --name "Russell" --email russell@example.com   # generates ~/.roboar/key, registers
roboar auth login                                                    # Ed25519 challenge → session token

roboar platform register --name "Russell's RPi 4" --type raspberry_pi --tags "lidar,outdoor"
# → prints platform_id + platform_token (token shown once)

roboar discover agents --online --platform-type raspberry_pi
```

## Robot workflow (Raspberry Pi / ROS2 host)

```bash
export ROBOAR_PLATFORM_TOKEN=ptk_...    # from `roboar platform register`
export ROBOAR_PLATFORM_ID=plt_...
export ROBOAR_REGISTRY_URL=https://reg.robotunnel.io/v1

roboar agent start --preset operations
# Registers an operations agent + baseline capabilities, then heartbeats every 30s.
```

## Library usage

```python
from roboar import PlatformAgent, Capability

agent_mgr = PlatformAgent(platform_token=..., platform_id=..., registry_url="https://reg.robotunnel.io/v1")
handle = agent_mgr.register_agent(name="lidar-agent", description="...", agent_type="sensor", version="1.0.0")
handle.register_capability(Capability(
    name="get_lidar_scan",
    description="Get LiDAR Point Cloud",
    interface_type="ros2_topic",
    permission="paid",
    pricing={"model": "per_call", "amount": 0.001},
    ros2={"node_name": "lidar_node", "topic_name": "/lidar/points", "message_type": "sensor_msgs/PointCloud2"},
))
agent_mgr.start()   # blocking heartbeat loop
```

Requires Python 3.9+ and `cryptography`.

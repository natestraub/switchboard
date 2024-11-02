# Switchboard Documentation

`switchboard` is a service that manages Docker containers based on Redis pub/sub messages. It's designed to work with GPU-enabled containers using the NVIDIA runtime.

## Prerequisites

- Docker with NVIDIA Container Runtime
- Redis server
- NVIDIA drivers installed on host
- NVIDIA Container Toolkit

## Usage

```bash
switchboard --image <docker-image> [--redis redis://localhost:6379/0] [--channel default-channel]
```

### Command Line Flags

- `--image` (required): Docker image to spawn containers from
- `--redis`: Redis URL (default: redis://localhost:6379/0)
- `--channel`: Redis pub/sub channel to monitor (default: default-channel)

## Redis Commands

The service listens for JSON messages on the specified Redis channel. Messages must follow this format:

### Start Container
```json
{
  "action": "start",
  "id": "<unique-identifier>",
  "env": {
    "KEY1": "value1",
    "KEY2": "value2"
  }
}
```
- `id`: Must be unique across all running containers
- `env`: Optional environment variables passed to the container

### Kill Container
```json
{
  "action": "kill",
  "id": "<unique-identifier>"
}
```

## GPU Configuration

The service automatically configures containers with:
- NVIDIA runtime
- Full GPU capabilities (`NVIDIA_DRIVER_CAPABILITIES=all`)
- Access to all available GPUs
- GPU compute, graphics, and utility capabilities

## Important Behaviors

- Containers are automatically removed after being stopped
- Duplicate container IDs are rejected
- The service maintains an in-memory mapping of IDs to containers
- State is not persisted across service restarts
- Each Redis message is processed concurrently
- The service handles graceful shutdown on SIGINT

## Logging

The service uses structured logging with the following levels:
- INFO: Container lifecycle events
- ERROR: Operation failures
- WARN: Non-critical issues
- DEBUG: Message reception details

## Example Usage

Start a container:
```bash
redis-cli PUBLISH channel-1 '{
  "action": "start",
  "id": "job123",
  "env": {
    "MODEL_PATH": "/models/v1",
    "BATCH_SIZE": "4"
  }
}'
```

Kill a container:
```bash
redis-cli PUBLISH channel-1 '{
  "action": "kill",
  "id": "job123"
}'
```
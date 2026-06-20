# SkybloomTD Docker Setup

This guide covers the current Docker Compose setup for local development and shared dev, staging, and production deployments. Local GPU OCR notes assume Windows WSL 2, but the compose profiles are the same on Linux servers.

## What Runs In Compose

The root compose stack starts these shared services:

- `reverse-proxy`: Nginx entrypoint at `http://localhost`
- `frontend`: Next.js app
- `game`: Vite/Phaser game client
- `document-content-api`: upload/status/document API
- `document-content-grpc`: document content gRPC service
- `document-content-worker`: OCR, upload, and indexing worker
- `game-service`: game websocket/API service
- `game-generation-worker`: level and quiz generation worker
- `user-service`: auth/user service
- `rabbitmq`: message queue for document and game jobs
- `redis`: document task-status and game generation-status Redis
- `game-redis`: map, quiz, and gameplay session cache Redis

## Prerequisites

Install these on Windows:

- Windows 11, or a recent Windows 10 build with WSL 2 support
- WSL 2 with Ubuntu, preferably Ubuntu 22.04 or 24.04
- Docker Desktop with the WSL 2 backend enabled
- An NVIDIA GPU supported by CUDA on WSL
- A current NVIDIA Windows display driver with WSL CUDA support

Install these inside WSL:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git gnupg
```

## NVIDIA Requirements for OCR

The document content worker uses PaddleOCR/PaddleX from this image:

```text
paddlepaddle/paddle:3.3.0-gpu-cuda13.0-cudnn9.13
```

The worker Dockerfile also installs Linux runtime libraries needed by OCR:

```text
libgl1
libglib2.0-0
```

For WSL, install the NVIDIA driver on Windows, not a Linux NVIDIA driver inside
WSL. NVIDIA documents that the Windows host driver provides the CUDA driver
stub inside WSL as `libcuda.so`, and Linux NVIDIA GPU drivers should not be
installed inside WSL.

### Docker Desktop Path

If you use Docker Desktop with the WSL 2 backend, Docker Desktop provides GPU
support for containers on Windows/WSL. After installing the Windows NVIDIA
driver, verify GPU access:

```bash
nvidia-smi
docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
```

If both commands show your GPU, the OCR worker should be able to use CUDA.

### Docker Engine Inside WSL Path

If you run Docker Engine directly inside WSL instead of Docker Desktop, install
the NVIDIA Container Toolkit in WSL:

```bash
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
  | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo service docker restart
```

Then verify:

```bash
docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
```

Do not install `cuda`, `cuda-drivers`, or Linux `nvidia-driver-*` packages in
WSL for this setup.

## Clone and Configure

Clone the repo inside the WSL filesystem, not under `/mnt/c`, for much better
Docker and file-watcher performance:

```bash
mkdir -p ~/projects
cd ~/projects
git clone https://github.com/zyangchuan/SkybloomTD.git skybloomtd
cd skybloomtd
```

Create one root env file for the profile you are running. Frontend, game, backend, worker, and proxy settings all live in this single root env file now. Do not create separate `frontend/app/.env.*` or `frontend/game/.env.*` files.

For local development:

```bash
cp .env.local.example .env.local
```

For shared environments, copy the matching example on the target host:

```bash
cp .env.dev.example .env.dev
cp .env.staging.example .env.staging
cp .env.production.example .env.production
```

Edit the copied env file and fill in the required values:

- Public app and websocket base URLs
- Browser-facing Supabase URL, publishable key, and redirect URL
- Supabase JWT settings
- Supabase S3-compatible storage credentials
- Postgres settings
- OpenAI API key

RabbitMQ, Redis, game Redis, queue names, and task/status cache TTLs are fixed in code for the Compose network. The services use the container names `rabbitmq`, `redis`, and `game-redis`.

## Compose Profiles

The root `docker-compose.yml` defines the shared service graph. Profile-specific override files select commands, env files, mounts, and local host ports. RabbitMQ and Redis are defined in the shared root compose file for every profile.

| Profile | Env file | Override file | Typical use |
| --- | --- | --- | --- |
| Local | `.env.local` | `docker-compose.local.yml` | Full local stack with bind mounts, RabbitMQ, Redis, and GPU OCR worker |
| Dev | `.env.dev` | `docker-compose.dev.yml` | Shared development server using Compose RabbitMQ/Redis and external database/storage |
| Staging | `.env.staging` | `docker-compose.staging.yml` | Production-like staging deployment with private Compose RabbitMQ/Redis |
| Production | `.env.production` | `docker-compose.production.yml` | Production deployment with private Compose RabbitMQ/Redis |

Build and start the local stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build
```

Run a shared environment in the background:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build -d
docker compose -f docker-compose.yml -f docker-compose.staging.yml up --build -d
docker compose -f docker-compose.yml -f docker-compose.production.yml up --build -d
```

Open locally:

- App: `http://localhost`
- Game dev server: `http://localhost:5173`
- API docs: `http://localhost/docs`
- OpenAPI YAML: `http://localhost/openapi.yaml`
- RabbitMQ management UI: `http://localhost:15672`

Default RabbitMQ credentials in the example env files:

```text
guest / guest
```

Stop the local stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml down
```

Stop and remove local volumes:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml down -v
```

## Document Worker Only

The full local app, including the GPU document worker, uses the root compose files. If you need to run only the GPU document worker on a staging or production GPU host, use the worker-specific compose files from `services/document-content`:

```bash
cd services/document-content
docker compose -f docker-compose.worker.yml -f docker-compose.worker.production.yml up --build -d
```

The worker override declares its own `env_file`, such as `.env.staging` or `.env.production`. Worker-only hosts still need reachable RabbitMQ and Redis endpoints; the full root stack provides those as Compose services named `rabbitmq` and `redis`.

## Common WSL Fixes

If the OCR worker cannot see the GPU:

```bash
nvidia-smi
docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
```

If `nvidia-smi` fails in WSL, update the Windows NVIDIA driver and run:

```powershell
wsl --update
wsl --shutdown
```

Then reopen Ubuntu.

If Docker cannot access the GPU but `nvidia-smi` works in WSL:

- Make sure Docker Desktop is using the WSL 2 backend.
- Make sure Docker Desktop WSL integration is enabled for your Ubuntu distro.
- If using Docker Engine inside WSL, reinstall/configure `nvidia-container-toolkit`.

If the app cannot reach Redis/RabbitMQ:

- Check `.env.local`.
- The services use hardcoded Compose hostnames: `rabbitmq`, `redis`, and `game-redis`.
- Recreate containers after env changes:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build --force-recreate
```

## References

- NVIDIA CUDA on WSL User Guide: https://docs.nvidia.com/cuda/wsl-user-guide/index.html
- NVIDIA Container Toolkit install guide: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html
- Docker Desktop GPU support: https://docs.docker.com/desktop/features/gpu/
- Docker Desktop WSL guidance: https://docs.docker.com/docker-for-windows/wsl/

# SkybloomTD Local Setup on Windows WSL

This guide is for running the full local app from a Windows machine using WSL 2,
Docker Compose, and the GPU OCR worker.

## What Runs Locally

The local compose stack starts:

- `reverse-proxy`: Nginx entrypoint at `http://localhost`
- `frontend`: Next.js app
- `game`: Vite/Phaser game client
- `document-content-api`: upload/status/document API
- `document-content-grpc`: document content gRPC service
- `document-content-worker`: OCR, upload, and indexing worker
- `game-service`: game websocket/API service
- `game-generation-worker`: level and quiz generation worker
- `user-service`: auth/user service
- `rabbitmq`: local message queue
- `redis`: document task-status and generation-status Redis
- `game-redis`: map/session cache Redis

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

Create local env files:

```bash
cp .env.local.example .env.local
cp frontend/app/.env.local.example frontend/app/.env.local
cp frontend/game/.env.local.example frontend/game/.env.local
```

Edit the copied env files and fill in the required values:

- Supabase URL and keys
- Supabase JWT settings
- Supabase S3-compatible storage credentials
- Postgres settings
- OpenAI API key

For local broker/status services, keep these values pointing at the compose
services:

```env
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
REDIS_URL=redis://redis:6379/0
GAME_STATUS_REDIS_URL=redis://redis:6379/0
GAME_MAP_REDIS_URL=redis://game-redis:6379/0
GAME_SESSION_REDIS_URL=redis://game-redis:6379/0
DOCUMENT_CONTENT_QUEUE=document.process
GAME_GENERATION_QUEUE=game.generation.generate
TASK_STATUS_TTL_SECONDS=604800
```

## Start the App

Build and start the local stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build
```

Open:

- App: `http://localhost`
- Game dev server: `http://localhost:5173`
- RabbitMQ management UI: `http://localhost:15672`

Default local RabbitMQ credentials:

```text
guest / guest
```

Run in the background:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build -d
```

Stop the stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml down
```

Stop and remove local volumes:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml down -v
```

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

If the app keeps using remote Redis/RabbitMQ unexpectedly:

- Check `.env.local`.
- The local compose override forces document and game backend services to use
  `rabbitmq` and `redis` inside the compose network.
- Recreate containers after env changes:

```bash
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build --force-recreate
```

## References

- NVIDIA CUDA on WSL User Guide: https://docs.nvidia.com/cuda/wsl-user-guide/index.html
- NVIDIA Container Toolkit install guide: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html
- Docker Desktop GPU support: https://docs.docker.com/desktop/features/gpu/
- Docker Desktop WSL guidance: https://docs.docker.com/docker-for-windows/wsl/

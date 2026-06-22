FROM paddlepaddle/paddle:3.3.0-gpu-cuda13.0-cudnn9.13

WORKDIR /app

ENV LD_LIBRARY_PATH=/usr/lib/wsl/lib:$LD_LIBRARY_PATH

COPY ocr_worker/requirements.txt .

RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

RUN apt-get update && apt-get install -y \
    libgl1 \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY ocr_worker ocr_worker

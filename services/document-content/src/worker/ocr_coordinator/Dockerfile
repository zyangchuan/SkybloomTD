FROM python:3.12-slim

WORKDIR /app

COPY ocr_coordinator/requirements.txt .

RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

COPY ocr_coordinator ocr_coordinator

FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e .

CMD ["celery", "-A", "workers.app", "worker", "--loglevel=info", "-B"]

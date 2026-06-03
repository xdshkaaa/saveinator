FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

CMD ["python", "-m", "workers.app", "worker", "--loglevel=info"]

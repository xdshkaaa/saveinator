FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg curl ca-certificates unzip \
    && curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e . \
    && deno --version \
    && python -c "import curl_cffi; import yt_dlp; print(yt_dlp.version.__version__)"

CMD ["celery", "-A", "workers.app", "worker", "--loglevel=info", "-B", "-c", "1", "--max-tasks-per-child=1"]

FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg curl ca-certificates unzip \
    && curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml .
COPY docker/entrypoint.sh /entrypoint.sh
COPY alembic.ini .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e . && chmod +x /entrypoint.sh \
    && deno --version \
    && python -c "import curl_cffi; import yt_dlp; print(yt_dlp.version.__version__)" \
    && yt-dlp --version

ENV USE_POLLING=true

ENTRYPOINT ["/entrypoint.sh"]
CMD ["python", "-m", "bot.main"]

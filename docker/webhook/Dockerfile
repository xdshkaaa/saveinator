FROM node:24-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
RUN npm install -g https://github.com/SwapnilSoni1999/spotify-dl

WORKDIR /app

COPY pyproject.toml .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e . --break-system-packages

ENV USE_POLLING=true

CMD ["python3", "-m", "bot.main"]

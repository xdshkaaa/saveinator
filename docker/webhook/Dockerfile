FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    git \
    curl \
    ca-certificates \
    && curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
RUN npm install -g https://github.com/SwapnilSoni1999/spotify-dl

WORKDIR /app

COPY pyproject.toml .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e .

ENV USE_POLLING=true

CMD ["python", "-m", "bot.main"]

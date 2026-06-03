FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

ENV USE_POLLING=false

CMD ["python", "-m", "bot.main"]

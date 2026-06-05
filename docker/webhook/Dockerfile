FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml .
COPY bot/ bot/
COPY workers/ workers/
COPY db/ db/
COPY locales/ locales/

RUN pip install --no-cache-dir -e .

ENV USE_POLLING=true

CMD ["python", "-m", "bot.main"]

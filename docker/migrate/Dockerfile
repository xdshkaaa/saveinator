# syntax=docker/dockerfile:1

FROM python:3.12-slim

WORKDIR /app

ENV PYTHONPATH=/app

RUN pip install --no-cache-dir \
    alembic==1.14.1 \
    sqlalchemy[asyncio]==2.0.36 \
    asyncpg==0.30.0 \
    greenlet==3.1.1

COPY alembic.ini ./
COPY db ./db

ENTRYPOINT ["alembic"]
CMD ["upgrade", "head"]

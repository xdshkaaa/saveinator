#!/bin/sh
set -e

# Apply pending database migrations before starting the service.
echo "Running alembic upgrade head..."
alembic upgrade head

# Run the main command (bot worker, webhook, etc.).
exec "$@"

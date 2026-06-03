# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# tech-stack
- Use Python 3.12, aiogram 3.x, yt-dlp, and ffmpeg as the core bot stack. Confidence: 0.85
- Use Celery with Redis as message broker and result backend for background download/conversion tasks. Confidence: 0.85
- Use PostgreSQL for production database and SQLite for local development, with SQLAlchemy 2.0 (async) + Alembic for migrations. Confidence: 0.85

# architecture
- Never run download or conversion inside the Telegram event loop; always delegate to background workers. Confidence: 0.85
- Implement per-user and per-chat rate limiting to prevent abuse. Confidence: 0.80
- Clean up temporary files immediately after sending, even on exceptions (use context managers, atexit handlers, or worker cleanup). Confidence: 0.80

# deployment
- Use Docker + docker-compose for containerization and systemd for auto-start on the VPS. Confidence: 0.85
- Use webhook mode with nginx reverse proxy for production, not long-polling. Confidence: 0.85

# i18n
- Default bot language is English; on first /start, present language picker with EN and RU options. Confidence: 0.70

# git
- Use git@github.com:pyfig/saveinator.git as the remote origin for this project. Confidence: 0.70

# communication-style
- When providing development plans, include concrete code snippets, callback data examples, and configuration samples throughout. Confidence: 0.75

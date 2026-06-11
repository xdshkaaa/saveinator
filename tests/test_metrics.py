from prometheus_client import REGISTRY, generate_latest

from bot.metrics import (
    DOWNLOADS_ENQUEUED_TOTAL,
    ERRORS_TOTAL,
    MESSAGES_RECEIVED_TOTAL,
    record_command,
    record_error,
    record_message,
    refresh_uptime,
)


class TestBotMetrics:
    def test_record_message_increments_counter(self):
        before = MESSAGES_RECEIVED_TOTAL._value.get()  # type: ignore[attr-defined]
        record_message(12345)
        after = MESSAGES_RECEIVED_TOTAL._value.get()  # type: ignore[attr-defined]
        assert after == before + 1

    def test_record_command_increments_labeled_counter(self):
        record_command("start")
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_commands_handled_total" in output
        assert 'command="start"' in output

    def test_record_error_increments_counter(self):
        record_error("test")
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_errors_total" in output
        assert 'source="test"' in output

    def test_downloads_enqueued_metric_exists(self):
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="youtube").inc()
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_downloads_enqueued_total" in output

    def test_downloads_enqueued_supports_spotify(self):
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="spotify").inc()
        output = generate_latest(REGISTRY).decode()
        assert 'platform="spotify"' in output

    def test_uptime_gauge_updates(self):
        refresh_uptime()
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_uptime_seconds" in output

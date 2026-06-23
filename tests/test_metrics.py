from prometheus_client import REGISTRY, generate_latest

from bot.metrics import (
    DOWNLOADS_ENQUEUED_TOTAL,
    ERRORS_TOTAL,
    HTTP_REQUESTS_TOTAL,
    MESSAGES_RECEIVED_TOTAL,
    USERS_CREATED_TOTAL,
    init_platform_metrics,
    record_command,
    record_error,
    record_message,
    record_user_created,
    refresh_uptime,
)
from workers.metrics import DOWNLOAD_FILE_SIZE_BYTES


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

    def test_downloads_enqueued_supports_soundcloud(self):
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="soundcloud").inc()
        output = generate_latest(REGISTRY).decode()
        assert 'platform="soundcloud"' in output

    def test_init_platform_metrics_exports_instagram(self):
        init_platform_metrics()
        output = generate_latest(REGISTRY).decode()
        assert 'platform="instagram"' in output

    def test_uptime_gauge_updates(self):
        refresh_uptime()
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_uptime_seconds" in output

    def test_user_created_metric_exists(self):
        before = USERS_CREATED_TOTAL._value.get()  # type: ignore[attr-defined]
        record_user_created()
        after = USERS_CREATED_TOTAL._value.get()  # type: ignore[attr-defined]
        assert after == before + 1

    def test_http_request_metric_exports_route_status(self):
        HTTP_REQUESTS_TOTAL.labels(
            method="POST",
            route="/download/pinterest",
            status="200",
        ).inc()
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_http_requests_total" in output
        assert 'route="/download/pinterest"' in output
        assert 'status="200"' in output

    def test_download_file_size_histogram_exists(self):
        DOWNLOAD_FILE_SIZE_BYTES.labels(platform="youtube").observe(5 * 1024 * 1024)
        output = generate_latest(REGISTRY).decode()
        assert "saveinator_download_file_size_bytes_bucket" in output
        assert 'platform="youtube"' in output

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DASHBOARD_DIR = ROOT / "monitoring" / "grafana" / "dashboards"
ALERTS_FILE = ROOT / "monitoring" / "prometheus" / "alerts.yml"


def _dashboard(name: str) -> dict:
    with (DASHBOARD_DIR / name).open() as fh:
        return json.load(fh)


def _panel_titles(dashboard: dict) -> set[str]:
    return {panel["title"] for panel in dashboard.get("panels", [])}


def _target_exprs(dashboard: dict) -> list[str]:
    exprs: list[str] = []
    for panel in dashboard.get("panels", []):
        for target in panel.get("targets", []):
            expr = target.get("expr")
            if expr:
                exprs.append(expr)
    return exprs


def test_required_operational_dashboards_are_provisioned():
    expected = {
        "saveinator-overview.json": "Saveinator Unified Monitoring",
        "operations.json": "Saveinator Operations",
        "downloads.json": "Download Operations",
        "worker-celery.json": "Worker / Celery",
        "user-activity.json": "User Activity",
        "reliability-errors.json": "Error & Reliability",
        "data-stores.json": "PostgreSQL & Redis",
        "logs.json": "Logs",
    }

    for filename, title in expected.items():
        dashboard = _dashboard(filename)
        assert dashboard["title"] == title
        assert dashboard["tags"]
        assert dashboard.get("refresh") == "30s"


def test_dashboards_cover_requested_monitoring_surfaces():
    expectations = {
        "operations.json": {
            "Bot Uptime",
            "Incoming Messages",
            "Commands by Type",
            "Active Chats",
            "Telegram API Latency p95",
            "Webhook Health",
        },
        "saveinator-overview.json": {
            "Downloads by Platform",
            "Telegram RPC Requests",
            "Users and Messages",
            "HTTP Handlers by Route",
            "HTTP Handler Latency p95",
        },
        "downloads.json": {
            "Downloads Enqueued by Platform",
            "Successful Downloads",
            "Failed Downloads",
            "Failure Rate by Platform",
            "Timeout Errors",
            "Top Failing Platforms",
        },
        "worker-celery.json": {
            "Worker Uptime",
            "Tasks Started",
            "Tasks Completed",
            "Tasks Failed",
            "Queue Backlog",
            "Memory Growth per Worker",
        },
        "user-activity.json": {
            "Active Users per Hour",
            "New Users per Day",
            "Most Used Commands",
            "Rate Limit Events",
            "Spam Middleware Events",
            "Banned User Messages",
        },
        "reliability-errors.json": {
            "Total Errors by Source",
            "Errors by Platform",
            "Database Errors",
            "Redis Errors",
            "Warning Logs",
            "Top Recurring Error Messages",
        },
        "data-stores.json": {
            "PostgreSQL Availability",
            "Active Connections",
            "Downloads Table Growth",
            "Redis Availability",
            "Redis Memory Usage",
            "Redis Keys",
        },
    }

    for filename, required_titles in expectations.items():
        assert required_titles <= _panel_titles(_dashboard(filename))


def test_logs_dashboard_has_investigation_filters_and_failure_panels():
    dashboard = _dashboard("logs.json")
    variable_names = {
        item["name"]
        for item in dashboard.get("templating", {}).get("list", [])
    }
    assert {
        "service",
        "container",
        "level",
        "platform",
        "user_id",
        "chat_id",
        "task_id",
    } <= variable_names

    titles = _panel_titles(dashboard)
    assert {
        "Recent Errors",
        "Recent Warnings",
        "Failed Download Logs",
        "Telegram API Failure Logs",
        "Worker Failure Logs",
        "Banned User Activity Logs",
    } <= titles


def test_dashboard_queries_use_prometheus_and_loki_datasources():
    all_exprs = "\n".join(
        expr
        for dashboard_path in DASHBOARD_DIR.glob("*.json")
        for expr in _target_exprs(_dashboard(dashboard_path.name))
    )
    assert "saveinator_messages_received_total" in all_exprs
    assert "saveinator_celery_tasks_total" in all_exprs
    assert "saveinator_http_requests_total" in all_exprs
    assert "saveinator_users_created_total" in all_exprs
    assert "pg_up" in all_exprs
    assert "redis_up" in all_exprs
    assert "{container=~" in all_exprs


def test_alerts_cover_infrastructure_and_product_failures_with_severities():
    text = ALERTS_FILE.read_text()
    alerts = set(re.findall(r"^\s+- alert: ([A-Za-z0-9_]+)", text, re.MULTILINE))
    expected_alerts = {
        "BotTargetDown",
        "WorkerTargetDown",
        "HighErrorRate",
        "TelegramAPIFailuresSpike",
        "NoMessagesForThirtyMinutes",
        "HighDownloadFailureRate",
        "HighTimeoutRate",
        "QueueBacklogTooHigh",
        "WorkerTaskDurationTooHigh",
        "LowDiskSpace",
        "PostgresDown",
        "RedisDown",
        "LokiNoLogsReceived",
        "PrometheusTargetDown",
    }
    assert expected_alerts <= alerts

    severities = set(re.findall(r"^\s+severity: (critical|warning|info)", text, re.MULTILINE))
    assert {"critical", "warning", "info"} <= severities

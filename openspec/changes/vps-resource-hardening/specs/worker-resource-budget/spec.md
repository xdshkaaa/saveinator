## ADDED Requirements

### Requirement: Asynq worker concurrency matches host CPU budget
The system SHALL run each asynq worker process (`saveinator`, `botd`) with a concurrency of 1, so that no more than one CPU-bound task (e.g. ffmpeg transcode) executes per process at a time on a 1 vCPU host.

#### Scenario: Two tasks queued on one process
- **WHEN** two download tasks are enqueued to the same worker process while it is idle
- **THEN** the process executes them one at a time, starting the second only after the first completes or fails

#### Scenario: Both processes active simultaneously
- **WHEN** `saveinator` and `botd` each have a task in flight at the same time
- **THEN** at most 2 tasks execute concurrently system-wide (1 per process), never more

### Requirement: Monitoring stack memory is bounded
The system SHALL declare an explicit memory limit on every container in `docker-compose.monitoring.yml`, so no monitoring container can grow unbounded and starve the application containers on a 4GB host.

#### Scenario: Monitoring container restarts under memory pressure
- **WHEN** a monitoring container's memory usage reaches its configured `mem_limit`
- **THEN** the container is constrained/restarted by the container runtime rather than consuming host memory needed by `saveinator`, `botd`, `db`, or `redis`

### Requirement: Monitoring footprint is minimized on single-core deployments
The system SHALL exclude `cadvisor`, `loki`, and `promtail` from the default monitoring stack, retaining only `prometheus`, `alertmanager`, `grafana`, `node_exporter`, `postgres_exporter`, and `redis_exporter`.

#### Scenario: Monitoring stack deployed
- **WHEN** `docker-compose.monitoring.yml` is deployed
- **THEN** no `cadvisor`, `loki`, or `promtail` container is created, and the six retained services start and expose metrics as before

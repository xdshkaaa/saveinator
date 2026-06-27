package metrics

import "time"

func ObserveTelegramAPI(method string, err error, started time.Time) {
	status := "ok"
	if err != nil {
		status = "error"
		TelegramAPIFailuresTotal.Inc()
	}
	TelegramAPIRequestsTotal.WithLabelValues(method, status).Inc()
	TelegramAPILatencySeconds.WithLabelValues(method).Observe(time.Since(started).Seconds())
}

func CallTelegram(method string, fn func() error) error {
	started := time.Now()
	err := fn()
	ObserveTelegramAPI(method, err, started)
	return err
}

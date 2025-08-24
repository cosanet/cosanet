package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/cosanet/cosanet/internal/collector"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type CliOpts struct {
	LogFormat     string
	ListenAddr    string
	CacheDuration time.Duration
	Verbosity     string
}

var (
	Version        = "v0.0.0"
	CommitHash     = "0000000"
	BuildTimestamp = "1970-01-01T00:00:00"
	Builder        = "go version go1.xx.y os/platform"
	ProjectURL     = "https://github.com/cosanet/cosanet"
)

// Very long story short: we can't collect other netns stats in goroutine
func main() {
	var logger *slog.Logger

	opts := &CliOpts{}
	flag.StringVar(&opts.LogFormat, "logformat", "json", "Log output format: json or text")
	flag.StringVar(&opts.ListenAddr, "listen", ":9100", "Address and port to listen on (e.g. :9100 or 0.0.0.0:9100)")
	flag.DurationVar(&opts.CacheDuration, "cache-duration", 500*time.Millisecond, "Cache duration for metrics collection (e.g. 500ms, 2s, 1m)")
	flag.StringVar(&opts.Verbosity, "verbosity", "info", "Log verbosity: debug, info, warn, error")
	flag.Parse()

	var logLevel slog.Level
	switch opts.Verbosity {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	if opts.LogFormat == "text" {
		handler := &PrettyHandler{out: os.Stdout}
		logger = slog.New(handler)
	} else {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	}

	slog.SetDefault(logger)
	slog.Info(
		"cosanet starting",
		slog.String("version", Version),
		slog.String("hash", CommitHash),
		slog.String("build_timestamp", BuildTimestamp),
		slog.String("builder", Builder),
		slog.String("project_url", ProjectURL),
	)

	hostname, err := os.Hostname()
	if err != nil {
		slog.Error("Failed to get hostname", slog.Any("err", err))
	} else {
		slog.Info("Hostname", slog.String("hostname", hostname))
	}

	// Part of the kludge to perform the collection on main thread (see bellow)
	collectRequestChan := make(chan collector.CollectRequest)
	collector := collector.NewCosanetCollector(
		hostname,
		collectRequestChan,
		collector.CosanetCollectorOptions{},
	)

	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html>
<head><title>Cosanet Exporter</title></head>
<body>
	<h1>Cosanet Exporter</h1>
	<p><a href="/metrics">Metrics</a></p>
</body>
</html>` + "\n"))
	})
	slog.Info("Exporter running", slog.String("address", opts.ListenAddr+"/metrics"))
	go func() {
		err = http.ListenAndServe(opts.ListenAddr, nil)
		if err != nil {
			slog.Error("Exporter failed", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var cacheTimestamp time.Time
	var metricsCache []prometheus.Metric

	for collectRequest := range collectRequestChan {
		if time.Since(cacheTimestamp) > opts.CacheDuration || len(metricsCache) == 0 {
			metricsChan := make(chan prometheus.Metric)
			metricTemp := []prometheus.Metric{}
			go func() {
				for m := range metricsChan {
					metricTemp = append(metricTemp, m)
				}
			}()
			collector.CollectFromMainThread(metricsChan)
			close(metricsChan)
			metricsCache = metricTemp
			cacheTimestamp = time.Now()
		}
		for _, m := range metricsCache {
			collectRequest.Feed <- m
		}
		collectRequest.Done <- true
	}

}

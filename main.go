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
	LogFormat        string
	ListenAddr       string
	CacheDuration    time.Duration
	Verbosity        string
	CollectorOptions collector.CosanetCollectorOptions
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

	// Generic settings
	flag.StringVar(
		&opts.LogFormat,
		"logformat",
		"json",
		"Log output format: json or text",
	)
	flag.StringVar(
		&opts.ListenAddr,
		"listen",
		":9156",
		"Address and port to listen on (e.g. :8080 or 0.0.0.0:9988)",
	)
	flag.DurationVar(
		&opts.CacheDuration,
		"cache-duration",
		500*time.Millisecond,
		"Cache duration for metrics collection (e.g. 500ms, 2s, 1m)",
	)
	flag.StringVar(
		&opts.Verbosity,
		"verbosity",
		"info",
		"Log verbosity: debug, info, warn, error",
	)

	// Collector settings

	// Pod filtering
	flag.StringVar(
		&opts.CollectorOptions.PodFilter,
		"collector.pod-filter",
		"^.+$",
		"filter namespace/pod based on regex (eg: ^default/.*$)",
	)

	// Host related
	flag.BoolVar(
		&opts.CollectorOptions.CollectHost.Enabled,
		"collector.host-metrics.enabled",
		true,
		"collect host metrics",
	)

	// Conntrack related
	flag.BoolVar(
		&opts.CollectorOptions.Conntrack.Enabled,
		"collector.connstrack.enabled",
		true,
		"enable conntack stats (curr and max) collection",
	)

	// SNMP related
	flag.BoolVar(
		&opts.CollectorOptions.Snmp.Enabled,
		"collector.snmp.enabled",
		true,
		"enable /proc/net/snmp and snmp6 collection",
	)
	flag.StringVar(
		&opts.CollectorOptions.Snmp.MetricInclude,
		"collector.snmp.metric-include",
		"^(Tcp_((Act|Pass)iveOpens|CurrEstab)|Ip6_(In|Out)Octets)$",
		"filter snmp metrics using regex tested against proto_metric",
	)

	// Netstat related
	flag.BoolVar(
		&opts.CollectorOptions.Netstat.Enabled,
		"collector.netstat.enabled",
		true,
		"enable /proc/net/netstat collection",
	)
	flag.StringVar(
		&opts.CollectorOptions.Netstat.MetricInclude,
		"collector.netstat.metric-include",
		"^IpExt_(In|Out)Octets$",
		"filter netstat metrics using regex tested against proto_metric",
	)

	// Socket Protocol related
	flag.BoolVar(
		&opts.CollectorOptions.SockProto.Enabled,
		"collector.sockproto.enabled",
		false,
		"enable per socket protocol states stats (/proc/net/{tcp,udp,icmp,udplite,raw}{,6}) (default false)",
	)
	flag.StringVar(
		&opts.CollectorOptions.SockProto.Protos,
		"collector.sockproto.protos",
		"tcp,udp",
		"socket protocol list to collect (comma separated, available: tcp, udp, icmp, udplite and raw)",
	)

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
		handler := &PrettyHandler{Out: os.Stdout, Level: logLevel}
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

	nodename := os.Getenv("NODE_NAME")
	if nodename == "" {
		var err error
		nodename, err = os.Hostname()
		if err != nil {
			slog.Error("Failed to get hostname", slog.Any("err", err))
		}
	}
	slog.Info("Nodename", slog.String("hostname", nodename))

	// Part of the kludge to perform the collection on main thread (see bellow)
	collectRequestChan := make(chan collector.CollectRequest)
	collector := collector.NewCosanetCollector(
		nodename,
		collectRequestChan,
		opts.CollectorOptions,
	)

	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html>
<head><title>Cosanet Exporter ` + Version + `</title></head>
<body>
	<h1>Cosanet Exporter ` + Version + `</h1>
	<p>Version: ` + Version + ` (` + CommitHash + `)</p>
	<p>Builder: ` + Builder + `</p>
	<p>Built on: ` + BuildTimestamp + `</p>
	<p>Project URL: ` + ProjectURL + `</p>
	<p><a href="/metrics">Metrics</a></p>
</body>
</html>` + "\n"))
	})
	slog.Info("Exporter running", slog.String("address", opts.ListenAddr+"/metrics"))
	go func() {
		err := http.ListenAndServe(opts.ListenAddr, nil)
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

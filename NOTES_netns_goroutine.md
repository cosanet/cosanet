# About Netns and go routine issue

To expose the problem, you'll notice the first call, from the go routine, shows the HOST metrics instead of each NetNS:

in `main.go`, before the `LockOSThread`:

```golang
    // Demonstrate the netns / goroutine issue
    go func() {
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()
        metricsChan := make(chan prometheus.Metric)
        go func() {
            for range metricsChan {
            }
        }()
        collector.CollectFromMainThread(metricsChan)
        close(metricsChan)
    }()

```

in `internal/collector/collector.go`, `collectStatsInNETNS`, Socket stats loop:

```golang
        for _, socktype := range []string{"tcp", "udp", "icmp", "udplite", "raw"} {

            // Demonstrate the netns / goroutine issue
            v4, v6, _ := c.collectAndEmitSockStats(info, socktype, ch)
            if socktype == "udp" {
                slog.Info(
                    socktype,
                    slog.Any("statv4", v4),
                    slog.Any("statv6", v6),
                    slog.String("namespace", info.Namespace),
                    slog.String("name", info.Name),
                    slog.String("netnsname", info.netNSName),
                )
            }
        }
```

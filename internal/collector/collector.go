package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cosanet/cosanet/internal/netstat"
	"github.com/cosanet/cosanet/internal/procnet_2l_parser"
	"github.com/cosanet/cosanet/internal/procnet_v6_parser"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/ti-mo/conntrack"
	"github.com/vishvananda/netns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type PodInfo struct {
	PID       int
	Name      string
	Namespace string
	netNSPath string
	netNSName string
}

type CosanetCollector struct {
	nodename            string
	chanToFeed          chan CollectRequest
	options             CosanetCollectorOptions
	podFilter           regexp.Regexp
	snmpMetricFilter    regexp.Regexp
	netstatMetricFilter regexp.Regexp
}

// Describe implements prometheus.Collector.
func (c *CosanetCollector) Describe(chan<- *prometheus.Desc) {
}

type CosanetCollectorOptions struct {
	PodFilter   string
	CollectHost struct {
		Enabled bool
	}
	Conntrack struct {
		Enabled bool
	}
	Snmp struct {
		Enabled       bool
		MetricInclude string
	}
	Netstat struct {
		Enabled       bool
		MetricInclude string
	}
	SockProto struct {
		Enabled bool
		Protos  string
	}
}

func NewCosanetCollector(nodename string, ch chan CollectRequest, options CosanetCollectorOptions) *CosanetCollector {
	return &CosanetCollector{
		nodename:            nodename,
		chanToFeed:          ch,
		options:             options,
		podFilter:           *regexp.MustCompile(options.PodFilter),
		snmpMetricFilter:    *regexp.MustCompile(options.Snmp.MetricInclude),
		netstatMetricFilter: *regexp.MustCompile(options.Netstat.MetricInclude),
	}
}

type CollectRequest struct {
	Done chan bool
	Feed chan<- prometheus.Metric
}

// The kludge to perform collect from main thread
func (c *CosanetCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now().UnixNano()
	doneCh := make(chan bool)
	defer close(doneCh)
	c.chanToFeed <- CollectRequest{Done: doneCh, Feed: ch}
	<-doneCh
	durationMs := float64(time.Now().UnixNano()-start) / 1e6
	slog.Info("CosanetCollector.Collect duration", slog.Float64("ms", durationMs))
}

// The kludge to perform collect from main thread
func (c *CosanetCollector) CollectFromMainThread(ch chan<- prometheus.Metric) {
	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current network namespace
	origns, _ := netns.Get()
	defer origns.Close()

	infos, err := listSandboxes()
	if err != nil {
		slog.Error("Failed to list sandboxes", slog.Any("err", err))
		os.Exit(1)
	}
	for _, info := range infos {
		composedPodName := fmt.Appendf(nil, "%s/%s", info.Namespace, info.Name)
		if !c.podFilter.Match(composedPodName) {
			slog.Debug(
				"sandbox skipped due to PodFilter",
				slog.String("name", info.Name),
				slog.String("namespace", info.Namespace),
				slog.String("composedpodname", string(composedPodName)),
				slog.String("filter", c.podFilter.String()),
			)
			continue
		}
		nsHandle, err := netns.GetFromPid(info.PID)
		if err != nil {
			slog.Error(
				"Failed to get network namespace for PID",
				slog.Int("pid", info.PID),
				slog.Any("err", err),
			)
			continue
		}

		if err := netns.Set(nsHandle); err != nil {
			slog.Error(
				"Failed to switch to network namespace",
				slog.Int("pid", info.PID),
				slog.Any("err", err),
			)
			nsHandle.Close()
			continue
		}

		c.collectStatsInNETNS(info, ch)
		if err := netns.Set(origns); err != nil {
			slog.Error(
				"Failed to switch back to the original network namespace",
				slog.Any("err", err),
			)
			os.Exit(1)
		}
		nsHandle.Close()
	}
	if c.options.CollectHost.Enabled {
		c.collectStatsInNETNS(
			PodInfo{
				Namespace: "HOST",
				netNSPath: "HOST",
				netNSName: "HOST",
			},
			ch,
		)
	}
}

func (c *CosanetCollector) collectStatsInNETNS(info PodInfo, ch chan<- prometheus.Metric) {

	if c.options.Conntrack.Enabled {
		dynamic_label_def := []string{
			"cosanet_node",
			"cosanet_pod",
			"cosanet_namespace",
			"cosanet_netnsname",
		}

		cntck, err := conntrack.Dial(nil)
		if err != nil {
			slog.Error("conntrack dial failed", slog.Any("err", err))
			os.Exit(1)
		}
		defer cntck.Close()

		statsg, _ := cntck.StatsGlobal()

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				"cosanet_conntrack_curr",
				"Number of entries in the conntrack table",
				dynamic_label_def,
				nil,
			),
			prometheus.UntypedValue,
			float64(statsg.Entries),
			c.nodename,
			info.Name,
			info.Namespace,
			info.netNSName,
		)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				"cosanet_conntrack_max",
				"Maximum entries in the conntrack table",
				dynamic_label_def,
				nil,
			),
			prometheus.UntypedValue,
			float64(statsg.MaxEntries),
			c.nodename,
			info.Name,
			info.Namespace,
			info.netNSName,
		)
	}

	// Socket stats per proto
	if c.options.SockProto.Enabled {
		sockprotoToCollect := strings.Split(c.options.SockProto.Protos, ",")
		for _, sockproto := range []string{"tcp", "udp", "icmp", "udplite", "raw"} {
			if !slices.Contains(sockprotoToCollect, sockproto) {
				slog.Debug(
					"socket proto skipped, not in collect list",
					slog.String("name", info.Name),
					slog.String("namespace", info.Namespace),
					slog.String("sockproto", sockproto),
					slog.Any("collectlist", sockprotoToCollect),
				)
				continue
			}
			c.collectAndEmitSockStats(info, sockproto, ch)
		}
	}

	if c.options.Snmp.Enabled {
		snmp_stats, _ := procnet_2l_parser.Parse2LFile("/proc/net/snmp")
		c.publishProcNet("snmp", snmp_stats, info, ch, c.snmpMetricFilter)

		snmp6_stats, _ := procnet_v6_parser.ParseV6File("/proc/net/snmp6")
		c.publishProcNet("snmp6", snmp6_stats, info, ch, c.snmpMetricFilter)
	}

	if c.options.Netstat.Enabled {
		netstat_stats, _ := procnet_2l_parser.Parse2LFile("/proc/net/netstat")
		c.publishProcNet("netstat", netstat_stats, info, ch, c.netstatMetricFilter)
	}

}

func (c *CosanetCollector) publishProcNet(source string, stats map[string]map[string]int, info PodInfo, ch chan<- prometheus.Metric, filter regexp.Regexp) {
	labels := []string{
		"cosanet_node",
		"cosanet_pod",
		"cosanet_namespace",
		"cosanet_netnsname",
	}
	for proto, metrics := range stats {
		for metric, value := range metrics {
			motif := fmt.Appendf(nil, "%s_%s", proto, metric)
			if !filter.Match(motif) {
				slog.Debug(
					"metric skipped due to filter",
					slog.String("name", info.Name),
					slog.String("namespace", info.Namespace),
					slog.String("proto_metric", string(motif)),
					slog.String("source", source),
					slog.Any("filter", filter.String()),
				)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					fmt.Sprintf("cosanet_proc_net_%s_%s_%s", source, proto, metric),
					fmt.Sprintf("/proc/net/%s %s %s entry", source, proto, metric),
					labels,
					nil,
				),
				prometheus.UntypedValue,
				float64(value),
				c.nodename,
				info.Name,
				info.Namespace,
				info.netNSName,
			)
		}
	}
}

type statscollcouple struct {
	v4 func() (netstat.SocketStats, error)
	v6 func() (netstat.SocketStats, error)
}

func (c *CosanetCollector) collectAndEmitSockStats(info PodInfo, socktype string, ch chan<- prometheus.Metric) (netstat.SocketStats, netstat.SocketStats, error) {
	var callbacks statscollcouple
	switch socktype {
	case "tcp":
		callbacks = statscollcouple{
			netstat.TCPStats,
			netstat.TCP6Stats,
		}
	case "udp":
		callbacks = statscollcouple{
			netstat.UDPStats,
			netstat.UDP6Stats,
		}

	case "icmp":
		callbacks = statscollcouple{
			netstat.ICMPStats,
			netstat.ICMP6Stats,
		}

	case "udplite":
		callbacks = statscollcouple{
			netstat.UDPLiteStats,
			netstat.UDPLite6Stats,
		}

	case "raw":
		callbacks = statscollcouple{
			netstat.RAWStats,
			netstat.RAW6Stats,
		}

	default:
		return nil, nil, fmt.Errorf("unrecognized socket type: %s", socktype)
	}

	statsv4, err := callbacks.v4()
	if err != nil {
		slog.Error("failed to collect IPv4 stats", slog.String("socktype", socktype), slog.Any("err", err))
		return nil, nil, err
	}

	statsv6, err := callbacks.v6()
	if err != nil {
		slog.Error("failed to collect IPv6 stats", slog.String("socktype", socktype), slog.Any("err", err))
		return nil, nil, err
	}

	labels := []string{
		"cosanet_node",
		"cosanet_pod",
		"cosanet_namespace",
		"cosanet_netnsname",
		"cosanet_state",
		"cosanet_ipversion",
	}
	for state, value := range statsv4 {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				fmt.Sprintf("cosanet_proc_net_%s", socktype),
				fmt.Sprintf("Socket statistics for %s", socktype),
				labels,
				nil,
			),
			prometheus.UntypedValue,
			float64(value),
			c.nodename,
			info.Name,
			info.Namespace,
			info.netNSName,
			state,
			"ipv4",
		)
	}

	for state, value := range statsv6 {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				fmt.Sprintf("cosanet_proc_net_%s", socktype),
				fmt.Sprintf("Socket statistics for %s", socktype),
				labels,
				nil,
			),
			prometheus.UntypedValue,
			float64(value),
			c.nodename,
			info.Name,
			info.Namespace,
			info.netNSName,
			state,
			"ipv6",
		)
	}

	return statsv4, statsv6, nil
}

type podSandboxStatusInfo struct {
	PID         int `json:"pid"`
	RuntimeSpec struct {
		Linux struct {
			Namespaces []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			}
		} `json:"linux"`
	} `json:"runtimeSpec"`
}

func (p *podSandboxStatusInfo) getNetworkNamespaceName() string {
	path := p.getNetworkNamespacePath()
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return path
	}
	return path[idx+1:]
}

func (p *podSandboxStatusInfo) getNetworkNamespacePath() string {
	for _, ns := range p.RuntimeSpec.Linux.Namespaces {
		if ns.Type == "network" {
			return ns.Path
		}
	}
	return "HOST"
}

func listSandboxes() ([]PodInfo, error) {
	// List of possible containerd socket paths
	socketPath, err := getCRISocketPath()
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("Failed to create gRPC client", slog.Any("err", err))
		return nil, err
	}
	defer conn.Close()

	client := criruntime.NewRuntimeServiceClient(conn)
	filter := &criruntime.PodSandboxFilter{
		State: &criruntime.PodSandboxStateValue{
			State: criruntime.PodSandboxState_SANDBOX_READY,
		},
	}
	req := &criruntime.ListPodSandboxRequest{Filter: filter}
	resp, err := client.ListPodSandbox(context.Background(), req)
	if err != nil {
		slog.Error("Failed to list pod sandboxes", slog.Any("err", err))
		return nil, err
	}

	sandboxes := resp.Items
	var podInfos []PodInfo

	for _, sb := range sandboxes {
		statusReq := &criruntime.PodSandboxStatusRequest{
			PodSandboxId: sb.Id,
			Verbose:      true,
		}
		statusResp, err := client.PodSandboxStatus(context.Background(), statusReq)
		if err != nil {
			slog.Error("Failed to get pod sandbox status", slog.Any("err", err))
			continue
		}

		jsonpayload_as_text := statusResp.Info["info"]

		var podInfo podSandboxStatusInfo
		err = json.Unmarshal([]byte(jsonpayload_as_text), &podInfo)
		if err != nil {
			slog.Warn("unable to unmarshal CRI's podInfo", slog.Any("err", err))
		}

		podInfos = append(podInfos, PodInfo{
			PID:       podInfo.PID,
			netNSPath: podInfo.getNetworkNamespacePath(),
			netNSName: podInfo.getNetworkNamespaceName(),
			Name:      statusResp.Status.Metadata.Name,
			Namespace: statusResp.Status.Metadata.Namespace,
		})
	}

	return podInfos, nil
}

func getCRISocketPath() (string, error) {
	socketPaths := []string{
		"/run/k3s/containerd/containerd.sock",
		"/var/run/containerd/containerd.sock",
		"/run/containerd/containerd.sock",
		"/var/run/dockershim.sock",
		"/run/crio/crio.sock",
	}

	if crisocket := os.Getenv("CRI_SOCKET"); crisocket != "" {
		slog.Info("searching for cri socket: using provided path", slog.String("path", crisocket))
		socketPaths = []string{crisocket}
	}

	for _, path := range socketPaths {
		if stat, err := os.Stat(path); err == nil {
			if stat.Mode()&os.ModeSocket != 0 {
				slog.Info("Found containerd socket", slog.String("path", path))
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no containerd socket file found in usual places or provided path %v", socketPaths)
}

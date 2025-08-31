# Cosanet Exporter - Container Sandbox Network Exporter

<p align="center"><a href="https://github.com/cosanet/cosanet" rel="cosanet"><img src="logo/cosanet_logo_256.png" alt="Cosanet Logo" width="256"></a></p>

Cosanet is a Prometheus exporter for collecting advanced network statistics from Linux hosts and Kubernetes pods. It is designed to operate in containerized environments and supports multi-namespace network statistics collection, including conntrack and /proc/net metrics.

## Why

The goal is to collect comprehensive network statistics from all container sandboxes without requiring instrumentation of each individual pods. This is achieved by deploying Cosanet as a DaemonSet, enabling centralized and efficient monitoring across the entire cluster.

## Features

- Collects network statistics from multiple network namespaces (pods/containers)
- Exposes metrics in Prometheus format on `/metrics` endpoint
- Supports conntrack table stats, `/proc/net/snmp`, `/proc/net/snmp6`, `/proc/net/netstat`
- Designed for use in Kubernetes clusters as DaemonSet

### Security considerations

Note that due to the way it works, it has security considerations:

- must be run with
  - `securityContext.privileged: true`
  - `hostPID: true`
- must be run as `root`
- have the node's CRI socket mounted eg: `/run/containerd/containerd.sock`
- have access to node's `proc` filesystem

## Architecture

Cosanet uses the [prometheus/client_golang](https://github.com/prometheus/client_golang) library to expose metrics. It leverages [vishvananda/netns](https://github.com/vishvananda/netns) to switch network namespaces and [ti-mo/conntrack](https://github.com/ti-mo/conntrack) for conntrack stats. The collector runs on the main OS thread to safely switch namespaces.

> [!IMPORTANT]
> Due to golang architecture, note the following
>
> - netns switch must be performed in the main thread which requires to be locked
> - To limit resource consumption on the main thread, as it can't be multi threaded, a metric cache has been implemented
> - Collecting some metrics like connection stats per proto can be relatively consuming, multiplied by the number of sandboxes, it can be quite expensive, act accordingly

## Metrics Exposed

Following metrics will be exposed by default on `:9156/metrics`:

- `cosanet_conntrack_curr`: Current entries in conntrack table
- `cosanet_conntrack_max`: Maximum entries in conntrack table
- `cosanet_proc_net_snmp_*`: SNMP stats from `/proc/net/snmp`
- `cosanet_proc_net_snmp6_*`: SNMPv6 stats from `/proc/net/snmp6`
- `cosanet_proc_net_netstat_*`: Netstat stats from `/proc/net/netstat`
- `cosanet_proc_net_<proto>`: per socket protocol states from `/proc/net/{tcp,udp,icmp,udplite,icmp}{,6}`

For detailed information about the available counters, see the official kernel documentation: [SNMP Counters](https://docs.kernel.org/networking/snmp_counter.html).

All metrics are labeled with:

- `cosanet_node`: Node name
- `cosanet_pod`: Pod name
- `cosanet_namespace`: Pod namespace
- `cosanet_netnsname`: Network namespace name (`HOST` for host network)

Per proto stats also have the following labels:

- `cosanet_ipversion`: `ipv4` or `ipv6`
- `cosanet_state`: `LISTEN`, `CLOSE`, `TIME_WAIT`, `ESTABLISHED` ...

If cosanet's service account has get, list, and watch permission on replicasets, jobs and pods across all namespaces then metrics label will also have:

- `cosanet_pod_controller_kind`
- `cosanet_pod_controller_name`

## Usage

## Configuration

Cosanet will auto-detect the container runtime socket. You can override the socket path by setting the `CRI_SOCKET` environment variable:

```bash
export CRI_SOCKET=/custom/path/to/containerd.sock
```

## Arguments

Cosanet Exporter supports the following command-line arguments:

| Argument                            | Default                                                                                                                      | Description                                                                                                     |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `-logformat`                        | `json`                                                                                                                       | Log output format: `json` or `text`                                                                             |
| `-listen`                           | `:9156`                                                                                                                      | Address and port to listen on (e.g. `:8080` or `0.0.0.0:9988`)                                                  |
| `-cache-duration`                   | `500ms`                                                                                                                      | Cache duration for metrics collection (e.g. `500ms`, `2s`, `1m`)                                                |
| `-verbosity`                        | `info`                                                                                                                       | Log verbosity: `debug`, `info`, `warn`, `error`                                                                 |
| `-collector.host-metrics.enabled`   | `true`                                                                                                                       | Collect host metrics                                                                                            |
| `-collector.connstrack.enabled`     | `true`                                                                                                                       | Enable conntrack stats (curr and max) collection                                                                |
| `-collector.snmp.enabled`           | `true`                                                                                                                       | Enable `/proc/net/snmp` and `snmp6` collection                                                                  |
| `-collector.snmp.metric-include`    | <code>^(Tcp_((Act&#124;Pass)iveOpens&#124;CurrEstab)&#124;Ip6_(In&#124;Out)Octets&#124;Udp6?_(In&#124;Out)Datagrams)$</code> | Filter SNMP metrics using regex tested against `<proto>_<metric>`                                               |
| `-collector.netstat.enabled`        | `true`                                                                                                                       | Enable `/proc/net/netstat` collection                                                                           |
| `-collector.netstat.metric-include` | <code>^IpExt_(In&#124;Out)Octets$</code>                                                                                     | Filter netstat metrics using regex tested against `<proto>_<metric>`                                            |
| `-collector.sockproto.enabled`      | `false`                                                                                                                      | Enable per socket protocol states stats (`/proc/net/{tcp,udp,icmp,udplite,raw}{,6}`, can be resource consuming) |
| `-collector.sockproto.protos`       | `tcp,udp`                                                                                                                    | Socket protocol list to collect, comma separated                                                                |
| `-collector.pod-filter`             | `^.+$`                                                                                                                       | Filter namespace/pod based on regex                                                                             |

Due to the large amount of metrics emitted per sandbox (~400+), default settings focus around trafic (In/OutOctets), UDP Datagrams (In/Out) and incoming (`PassiveOpens`), outgoing (`ActiveOpens`) and established (`CurrEstab`) TCP connection.

Example usage:

```bash
./cosanet \
  -listen=:9156 \
  -verbosity=debug \
  -collector.perproto.enable=1 \
  -collector.pod-filter="^default/.*$" \
  -collector.netstat.metric-include ^Tcp \
  -collector.host-metrics.enabled=f \
  -collector.snmp.metric-include Udp6?_
```

## Available Metrics

Below is a list of metrics exposed by Cosanet, grouped by their source:

### Conntrack Table

- `cosanet_conntrack_curr`
- `cosanet_conntrack_max`

### /proc/net/netstat

- `cosanet_proc_net_netstat_IpExt_*`
- `cosanet_proc_net_netstat_MPTcpExt_*`
- `cosanet_proc_net_netstat_TcpExt_*`

### /proc/net/snmp

- `cosanet_proc_net_snmp_IcmpMsg_*`
- `cosanet_proc_net_snmp_Icmp_*`
- `cosanet_proc_net_snmp_Ip_*`
- `cosanet_proc_net_snmp_Tcp_*`
- `cosanet_proc_net_snmp_Udp_*`
- `cosanet_proc_net_snmp_UdpLite_*`

### /proc/net/snmp6

- `cosanet_proc_net_snmp6_Icmp6_*`
- `cosanet_proc_net_snmp6_Ip6_*`
- `cosanet_proc_net_snmp6_Udp6_*`
- `cosanet_proc_net_snmp6_UdpLite6_*`

### Socket Protocol States

- `cosanet_proc_net_tcp`
- `cosanet_proc_net_udp`
- `cosanet_proc_net_udplite`
- `cosanet_proc_net_icmp`
- `cosanet_proc_net_raw`

> For a full list of metric names, see the [metrics file](metrics.md).

## Development

### Prerequisites

- Go 1.25+
- Linux (requires network namespace support)
- k3s cluster locally

### Building Locally

```bash
# No args will build both or you can specify the build you want
./build.sh local
# - or -
./build.sh docker
```

### Run Script

```bash
docker run --rm -it --privileged \
    -v /run/k3s/containerd/containerd.sock:/run/containerd/containerd.sock:ro \
    -v /proc:/proc:ro \
    --name cosanet \
    cosanet:latest
```

## License

This project is licensed under the MIT License.

## References

- [Prometheus](https://prometheus.io/)
- [Go](https://golang.org/)
- [conntrack](https://github.com/ti-mo/conntrack)
- [vishvananda/netns](https://github.com/vishvananda/netns)
- [cakturk/go-netstat](https://github.com/cakturk/go-netstat)

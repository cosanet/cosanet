# Cosanet Exporter - Container Sandbox Network Exporter

<p align="center"><a href="https://github.com/cosanet/cosanet" rel="cosanet"><img src="logo/cosanet_logo_256.png" alt="Cosanet Logo" width="256"></a></p>


Cosanet is a Prometheus exporter for collecting advanced network statistics from Linux hosts and Kubernetes pods. It is designed to operate in containerized environments and supports multi-namespace network statistics collection, including conntrack and /proc/net metrics.

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

All metrics are labeled with:

- `cosanet_node`: Node name
- `cosanet_pod`: Pod name
- `cosanet_namespace`: Pod namespace

## Usage

## Configuration

Cosanet will auto-detect the container runtime socket. You can override the socket path by setting the `CRI_SOCKET` environment variable:

```bash
export CRI_SOCKET=/custom/path/to/containerd.sock
```

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

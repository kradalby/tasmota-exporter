# tasmota-exporter

A [Prometheus](prometheus.io) exporter that translates between Prometheus and (some) [Tasmota](https://tasmota.github.io/docs/) power sockets.

It is implemented in a very similar way to [blackbox_exporter](https://github.com/prometheus/blackbox_exporter) which allows you to run it
as a "proxy" querying the power socket on the go and returns specific metrics for that power socket using targets and relabeling.

## Tested with

- Avatar UK 10A
- Athom Plug V2

## Configuration

tasmota-exporter does not need any configuration itself, and the seperation of power sockets are fully hosted in the Prometheus
scrape config.

Just run the binary somewhere where it can reach the power sockets over IP and from Prometheus.
By default, port `9090` will be used, but this can be changed `TASMOTA_EXPORTER_LISTEN_ADDR` on
the format `:9111`.

In Prometheus, add a new scrape target:

```yaml
scrape_configs:
  - job_name: tasmota
    metrics_path: /probe
    static_configs:
      - targets:
          # add the address of your sockets here
          - 10.0.0.3
          - livingroom-socket.local
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9090 # address of exporter
```

I recommend to have DNS names assigned to your sockets so the instance name will be human readable.

## Similar work

There is a couple of exporters for Tasmota already, but they did not fulfill all my critierias:

- [dyrkin/tasmota-exporter](https://github.com/dyrkin/tasmota-exporter)
  - Subscribes to an MQTT server where the Tasmota power sockets publish data
  - Pros
    - "service discovery" by picking up all sockets present in MQTT
  - Cons
    - The "freshness" of data depends on MQTT, not Prometheus scrape rate
    - My sockets seem to push data to another subtopic than supported
    - One more moving part that can fail (MQTT), hard to discover if socket breaks
- [astr0n8t/tasmota-power-exporter](https://github.com/astr0n8t/tasmota-power-exporter)
  - Sets up one exporter that parses data from `http://powersocket?m`
  - Pros
    - Queries the Tasmota power socket when Prometheus scrapes it
  - Cons
    - Only support one socket, one exporter per socket needed

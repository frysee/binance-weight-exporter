# Binance API weight limit exporter for Prometheus

## Configuratiom
- Metrics are published on port 9133 and under `/metrics`.
- To change port/telemetry path, pass them per command line: `-web.listen-address :9123 -web.telemetry-path /mymetrics`
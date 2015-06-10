# New Relic Exporter

Prometheus exporter for New Relic data.
Requires a New Relic account.

## Building and running

    go build
    ./newrelic_exporter <flags>

### Flags

Name               | Description
-------------------|------------
api.key            | API key
api.server         | API location.  Defaults to https://api.newrelic.com
api.period         | Period of data to request, in seconds.  Defaults to 60.
web.listen-address | Address to listen on for web interface and telemetry.  Port defaults to 9126.
web.telemetry-path | Path under which to expose metrics.

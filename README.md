# New Relic Exporter

Prometheus exporter for New Relic data.
Requires a New Relic account.

## Building and running

### Running in a container

    cp newrelic_exporter.yml.example newrelic_exporter.yml
	docker run mrf/newrelic-exporter

### From source

	git clone https://github.com/mrf/newrelic_exporter.git --branch release
	cd newrelic_exporter
    make
    cp newrelic_exporter.yml.example newrelic_exporter.yml
    ./newrelic_exporter

## Configuration Values 

Name               | Description
-------------------|------------
api.key            | API key
api.server         | API location.  Defaults to https://api.newrelic.com
api.period         | Period of data to request, in seconds.  Defaults to 60.
api.timeout        | Period of time to wait for an API response in seconds (default 5s)
web.listen-address | Address to listen on for web interface and telemetry.  Port defaults to 9126.
web.telemetry-path | Path under which to expose metrics.

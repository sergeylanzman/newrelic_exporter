module mfr/newrelic_exporter

go 1.18

replace github.com/Sirupsen/logrus v1.8.1 => github.com/sirupsen/logrus v1.8.1

require (
	github.com/antonholmquist/jason v1.0.0
	github.com/mrf/newrelic_exporter v0.0.0-20200405231859-09b48d7378c8
	github.com/prometheus/client_golang v1.12.2
	github.com/prometheus/log v0.0.0-20151026012452-9a3136781e1f
	github.com/tomnomnom/linkheader v0.0.0-20180905144013-02ca5825eb80
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Sirupsen/logrus v1.8.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
)

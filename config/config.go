package config

import (
	"time"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"github.com/prometheus/log"
)

type Config struct {
	// NewRelic related settings
	NRApiKey		string			`yaml:"api.key"`
	NRApiServer		string			`yaml:"api.server"`
	NRPeriod		int			`yaml:"api.period"`
	NRTimeout		time.Duration		`yaml:"api.timeout"`
	NRService		string			`yaml:"api.service"`
	NRApps			[]Application		`yaml:"api.apps"`
	NRMetricFilters		[]string		`yaml:"api.metric-filters"`
	NRValueFilters		[]string		`yaml:"api.value-filters"`

	// Prometheus Exporter related settings
	MetricPath		string			`yaml:"web.telemetry-path"`
	ListenAddress		string			`yaml:"web.listen-address"`
}

type Application struct {
	Id		int			`yaml:"id"`
	Name		string			`yaml:"name"`
}

func GetConfig(path string) (Config, error) {
	config := Config{}
	configSource, err := ioutil.ReadFile(path)
	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(configSource, &config)
	if err != nil {
		return config, err
	}

	log.Debugf("Config loaded: %v", config)

	return config, err
}
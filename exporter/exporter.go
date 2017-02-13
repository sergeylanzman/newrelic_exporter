package exporter

import (
	"time"
	"sync"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"

	"camino.ru/newrelic_exporter/config"
	"camino.ru/newrelic_exporter/newrelic"
)

// Namespace for metrics
const NameSpace = "newrelic"

// Last times of requesting apps list and metric names. Used for caching
var appListLastTime, metricNamesLastTime time.Time

var apps newrelic.AppList
var names newrelic.MetricNames

type Metric struct {
	App   string
	Name  string
	Value float64
	Label string
}

type Exporter struct {
	mu              sync.Mutex
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	metrics         map[string]prometheus.GaugeVec
	api             *newrelic.API
	cfg config.Config
}

func NewExporter(api *newrelic.API, cfg config.Config) *Exporter {
	return &Exporter{
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      "exporter_last_scrape_duration_seconds",
			Help:      "The last scrape duration.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: NameSpace,
			Name:      "exporter_scrapes_total",
			Help:      "Total scraped metrics",
		}),
		error: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      "exporter_last_scrape_error",
			Help:      "The last scrape error status.",
		}),
		metrics: map[string]prometheus.GaugeVec{},
		api: api,
		cfg: cfg,
	}
}

func (e *Exporter) scrape(ch chan <- Metric) {
	e.error.Set(0)
	e.totalScrapes.Inc()

	startTime := time.Now()
	log.Infof("Starting new scrape at %v.", startTime)

	if time.Since(appListLastTime) >= e.cfg.NRAppListCacheTime {
		err := apps.Get(e.api)
		if err != nil {
			log.Error(err)
			e.error.Set(1)
		} else {
			// Only successful tries should touch cache times
			appListLastTime = time.Now()
			log.Debugf("Application list updated at %v", appListLastTime)
		}
	} else {
		log.Debug("Applications list taken from cache")
	}

	for _, app := range apps.Applications {
		for name, value := range app.AppSummary {
			ch <- Metric{
				App:   app.Name,
				Name:  name,
				Value: value,
				Label: "application_summary",
			}
		}

		for name, value := range app.UsrSummary {
			ch <- Metric{
				App:   app.Name,
				Name:  name,
				Value: value,
				Label: "end_user_summary",
			}
		}
	}

	var wg sync.WaitGroup

	for i := range apps.Applications {
		app := apps.Applications[i]

		wg.Add(1)
		api := e.api

		go func() {
			defer wg.Done()

			var err error

			if time.Since(metricNamesLastTime) >= e.cfg.NRAppListCacheTime {
				// Getting metric names
				names = make(newrelic.MetricNames)

				err = names.Get(api, app.ID)
				log.Infof("Scraped %v metric names for app %v", len(names), app.ID)
				if err != nil {
					log.Error(err)
					e.error.Set(1)
				} else {
					// Only successful tries should touch cache times
					metricNamesLastTime = time.Now()
					log.Debugf("Metric names list updated at %v", appListLastTime)
				}
			} else {
				log.Debug("Metrics names list taken from cache")
			}

			// Getting metric data
			var data newrelic.MetricData

			err = data.Get(api, app.ID, names)
			log.Infof("Scraped %v metric datas for app %v", len(data.Metric_Data.Metrics), app.ID)
			if err != nil {
				log.Error(err)
				e.error.Set(1)
			}

			// Sending metrics
			for _, set := range data.Metric_Data.Metrics {
				if len(set.Timeslices) == 0 {
					continue
				}

				// As we set summarise=true there will only be one timeseries.
				for name, value := range set.Timeslices[0].Values {
					if v, ok := value.(float64); ok {
						ch <- Metric{
							App:   app.Name,
							Name:  name,
							Value: v,
							Label: set.Name,
						}
					}
				}
			}
		}()
	}

	wg.Wait()

	close(ch)
	e.duration.Set(float64(time.Now().UnixNano() - startTime.UnixNano()) / 1000000000)
	log.Infof("Scrape finished in %v", time.Since(startTime))
}

func (e *Exporter) recieve(ch <-chan Metric) {
	for metric := range ch {
		id := fmt.Sprintf("%s_%s", NameSpace, metric.Name)

		if m, ok := e.metrics[id]; ok {
			m.WithLabelValues(metric.App, metric.Label).Set(metric.Value)
		} else {
			g := prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: NameSpace,
					Name:      metric.Name,
				},
				[]string{"app", "component"})

			e.metrics[id] = *g
		}
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, m := range e.metrics {
		m.Describe(ch)
	}

	ch <- e.duration.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.error.Desc()
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Align requests to minute boundary.
	// As time.Round rounds to the nearest integar rather than floor or ceil,
	// subtract 30 seconds from the time before rounding.
	e.api.To = time.Now().Add(-time.Second * 30).Round(time.Minute).UTC()
	e.api.From = e.api.To.Add(-time.Duration(e.api.Period) * time.Second)

	metricChan := make(chan Metric)

	go e.scrape(metricChan)

	e.recieve(metricChan)

	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error

	for _, m := range e.metrics {
		m.Collect(ch)
	}
}

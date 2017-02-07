package newrelic

import (
	"fmt"
	"encoding/json"
	"bytes"
	"io"
	"strconv"
	"net/url"
	"time"
	"net/http"
	"crypto/tls"
	"io/ioutil"

	"github.com/prometheus/log"
	"github.com/tomnomnom/linkheader"

	"camino.ru/newrelic_exporter/config"
)

// Module version
const Version = "0.1b"

// User-Agent string
const UserAgent = "Prometheus-NewRelic-Exporter/" + Version

// Chunk size of metric requests
const ChunkSize = 10

var cfg config.Config

type API struct {
	server          url.URL
	apiKey          string
	service		string
	From            time.Time // remove
	To              time.Time // remove
	Period          int // remove
	unreportingApps bool
	client          *http.Client
}

func NewAPI(c config.Config) *API {
	cfg = c

	serverURL, err := url.Parse(cfg.NRApiServer)
	if err != nil {
		log.Fatal("Could not parse API URL: ", err)
	}
	if cfg.NRApiKey == "" {
		log.Fatal("Cannot continue without an API key.")
	}
	if cfg.NRService == "" {
		log.Fatal("Cannot continue without NewRelic service selected")
	}

	client := &http.Client{Timeout: cfg.NRTimeout}

	debugMode := true
	if debugMode {
		proxyUrl, _ := url.Parse("https://localhost:8888")
		transport := &http.Transport{}
		transport.Proxy = http.ProxyURL(proxyUrl)
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client.Transport = transport
	}

	return &API{
		server: *serverURL,
		apiKey: cfg.NRApiKey,
		service: cfg.NRService,
		client: client,
		Period: cfg.NRPeriod,
	}
}

func (a *API) req(path string, params string) ([]byte, error) {
	u, err := url.Parse(a.server.String() + path)
	if err != nil {
		return nil, err
	}
	u.RawQuery = params

	log.Debug("Making API call: ", u.String())

	req := &http.Request{
		Method: "GET",
		URL:    u,
		Header: http.Header{
			"User-Agent": {UserAgent},
			"X-Api-Key":  {a.apiKey},
		},
	}

	var data []byte

	return a.httpget(req, data)
}

func (a *API) httpget(req *http.Request, in []byte) (out []byte, err error) {
	resp, err := a.client.Do(req)
	if err != nil {
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	resp.Body.Close()
	out = append(in, body...)

	// Read the link header to see if we need to read more pages.
	links := linkheader.Parse(resp.Header.Get("Link"))

	relLast := links.FilterByRel("last")
	if len(relLast) > 0 {
		u, err := url.Parse(relLast[0].URL)
		if err != nil {
			log.Errorf("Error parsing 'last' relation link. %v", err)
		}
		log.Debugf("Found %v pages for %s", u.Query().Get("page"), req.URL)
	}

	relNext := links.FilterByRel("next")
	if len(relNext) > 0 {
		u := new(url.URL)

		u, err = url.Parse(relNext[0].URL)
		if err != nil {
			return
		}
		cursor := u.Query().Get("cursor")

		if cursor != "" {
			query := req.URL.Query()

			query.Set("cursor", cursor)
			query.Del("page")

			req.URL.RawQuery = query.Encode()

			return a.httpget(req, out)
		}
	}

	return
}

type AppList struct {
	Applications []Application
}

type Application struct {
	ID         int                `yaml:"id"`
	Name       string             `yaml:"name"`
	Health     string             `json:"health_status"`
	AppSummary map[string]float64 `json:"application_summary"`
	UsrSummary map[string]float64 `json:"end_user_summary"`
}

func (a *AppList) Get(api *API) error {
	if len(cfg.NRApps) > 0 {
		// Using local app list instead of getting it from API - one API call less
		log.Infof("Getting application list from config: %v", cfg.NRApps)
		for _, app := range cfg.NRApps {
			a.Applications = append(a.Applications, Application{ID: app.Id, Name: app.Name})
		}
		return nil
	}

	log.Infof("Requesting application list from %s.", api.server.String())
	body, err := api.req(fmt.Sprintf("/v2/%s.json", api.service), "")
	if err != nil {
		log.Error("Error getting application list: ", err)
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		page := new(AppList)
		if err := dec.Decode(page); err == io.EOF {
			break
		} else if err != nil {
			log.Error("Error decoding application list: ", err)
			return err
		}

		a.Applications = append(a.Applications, page.Applications...)
	}

	return nil
}

type MetricNames map[string][]string

func (m *MetricNames) Get(api *API, appID int) error {
	log.Infof("Requesting metrics names for application id %d.", appID)
	path := fmt.Sprintf("/v2/%s/%s/metrics.json", api.service, strconv.Itoa(appID))

	// We will only make filtered requests for metric names. Otherwise there are too many of them (tens of thousands)
	var i int
	var filter string
	channel := make(chan MetricNames)

	for i, filter = range cfg.NRMetricFilters {
		go func(filter string, ch chan<- MetricNames, counter int) error {
			params := url.Values{}
			params.Add("name", filter)

			body, err := api.req(path, params.Encode())
			if err != nil {
				log.Error("Error getting metric names: ", err)
				return err
			}

			dec := json.NewDecoder(bytes.NewReader(body))

			metricNamesBuffer := make(MetricNames)
			var lastKey string
			var curName string
			var curValues []string

			for {
				t, err := dec.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Fatal(err)
				}

				if t == "metrics" || t == "name" || t == "values" {
					lastKey = t.(string)
					continue
				}

				if _, ok := t.(json.Delim); ok {
					continue
				}

				if lastKey == "name" {
					curName = t.(string)
				} else if lastKey == "values" {
					curValues = append(curValues, t.(string))
				}

				if !dec.More() && lastKey == "values" {
					// finalizing metric addition
					metricNamesBuffer[curName] = curValues
					curValues = curValues[0:0]
				}
			}

			ch <- metricNamesBuffer

			return nil
		}(filter, channel, i)
	}

	for {
		mnBuffer := <-channel
		mm := *m
		for mn, mv := range mnBuffer {
			mm[mn] = mv
		}

		if i--; i < 0 {
			break
		}
	}

	return nil
}

type MetricData struct {
	Metric_Data struct {
		Metrics []struct {
			Name       string
			Timeslices []struct {
				Values map[string]interface{}
			}
		}
	}
}

func (m *MetricData) Get(api *API, appID int, names MetricNames) error {
	path := fmt.Sprintf("/v2/%s/%s/metrics/data.json", api.service, strconv.Itoa(appID))

	var nameList []string

	for name, _ := range names {
		// We urlencode the metric names as the API will return
		// unencoded names which it cannot read
		nameList = append(nameList, name)
	}
	log.Infof("Requesting %d metrics for application id %d.", len(nameList), appID)

	// Because the Go client does not yet support 100-continue
	// ( see issue #3665 ),
	// we have to process this in chunks, to ensure the response
	// fits within a single request.

	chans := make([]chan MetricData, 0)

	for i := 0; i < len(nameList); i += ChunkSize {

		chans = append(chans, make(chan MetricData))

		var thisList []string

		if i+ChunkSize > len(nameList) {
			thisList = nameList[i:]
		} else {
			thisList = nameList[i : i+ChunkSize]
		}

		go func(names []string, ch chan<- MetricData) {

			var data MetricData

			params := url.Values{}

			for _, thisName := range names {
				params.Add("names[]", thisName)
			}

			if len(cfg.NRValueFilters) > 0 {
				for _, valueFilter := range cfg.NRValueFilters {
					params.Add("values[]", valueFilter)
				}
			}

			params.Add("raw", "true")
			params.Add("summarize", "true")
			params.Add("period", strconv.Itoa(api.Period))
			params.Add("from", api.From.Format(time.RFC3339))
			params.Add("to", api.To.Format(time.RFC3339))

			body, err := api.req(path, params.Encode())
			if err != nil {
				log.Error("Error requesting metrics: ", err)
				close(ch)
				return
			}

			dec := json.NewDecoder(bytes.NewReader(body))
			for {

				page := new(MetricData)
				if err := dec.Decode(page); err == io.EOF {
					break
				} else if err != nil {
					log.Error("Error decoding metrics data: ", err)
					close(ch)
					return
				}

				data.Metric_Data.Metrics = append(data.Metric_Data.Metrics, page.Metric_Data.Metrics...)

			}

			ch <- data
			close(ch)

		}(thisList, chans[len(chans)-1])

	}

	allData := m.Metric_Data.Metrics

	for _, ch := range chans {
		m := <-ch
		allData = append(allData, m.Metric_Data.Metrics...)
	}
	m.Metric_Data.Metrics = allData

	return nil
}

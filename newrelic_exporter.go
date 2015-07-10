package main

// TODO: implement JSON parser that loops through the output from api.Get()

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

// Chunk size of metric requests
const ChunkSize = 10

// Namespace for metrics
const NameSpace = "newrelic"

// User-Agent string
const UserAgent = "NewRelic Exporter"

// This is to support skipping verification for testing and
// is deliberately not exposed to the user
var TlsIgnore bool = false

// Regular expression to parse Link headers
var rexp = `<([[:graph:]]+)>; rel="next", <([[:graph:]]+)>; rel="last"`
var LinkRexp *regexp.Regexp

func init() {
	LinkRexp = regexp.MustCompile(rexp)
}

type Metric struct {
	App   string
	Name  string
	Value float64
	Label string
}

type AppList struct {
	Applications []struct {
		Id         int
		Name       string
		Health     string             `json:"health_status"`
		AppSummary map[string]float64 `json:"application_summary"`
		UsrSummary map[string]float64 `json:"end_user_summary"`
	}
}

func (a *AppList) get(api newRelicApi) error {
	log.Debugf("Requesting application list from %s.", api.server.String())
	body, err := api.req("/v2/applications.json", "")
	if err != nil {
		log.Print("Error getting application list: ", err)
		return err
	}

	err = json.Unmarshal(body, a)
	return err
}

func (a *AppList) sendMetrics(ch chan<- Metric) {
	for _, app := range a.Applications {
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
}

type MetricNames struct {
	Metrics []struct {
		Name   string
		Values []string
	}
}

func (m *MetricNames) get(api newRelicApi, appId int) error {
	log.Debugf("Requesting metrics names for application id %d.", appId)
	path := fmt.Sprintf("/v2/applications/%s/metrics.json", strconv.Itoa(appId))

	body, err := api.req(path, "")
	if err != nil {
		log.Print("Error getting metric names: ", err)
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(body))

	for {
		var part MetricNames
		if err = dec.Decode(&part); err == io.EOF {
			break
		} else if err != nil {
			log.Print("Error decoding metric names: ", err)
			return err
		}
		tmpMetrics := append(m.Metrics, part.Metrics...)
		m.Metrics = tmpMetrics
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

func (m *MetricData) get(api newRelicApi, appId int, names MetricNames) error {
	path := fmt.Sprintf("/v2/applications/%s/metrics/data.json", strconv.Itoa(appId))

	var nameList []string

	for i := range names.Metrics {
		// We urlencode the metric names as the API will return
		// unencoded names which it cannot read
		nameList = append(nameList, names.Metrics[i].Name)
	}
	log.Debugf("Requesting %d metrics for application id %d.", len(nameList), appId)

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

			for _, thisName := range thisList {
				params.Add("names[]", thisName)
			}

			params.Add("raw", "true")
			params.Add("summarize", "true")
			params.Add("period", strconv.Itoa(api.period))
			params.Add("from", api.from.Format(time.RFC3339))
			params.Add("to", api.to.Format(time.RFC3339))

			body, err := api.req(path, params.Encode())
			if err != nil {
				log.Print("Error requesting metrics: ", err)
				close(ch)
				return
			}

			err = json.Unmarshal(body, &data)
			if err != nil {
				log.Print("Error decoding metrics data: ", err)
				close(ch)
				return
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

func (m *MetricData) sendMetrics(ch chan<- Metric, app string) {
	for _, set := range m.Metric_Data.Metrics {

		if len(set.Timeslices) == 0 {
			continue
		}

		// As we set summarise=true there will only be one timeseries.
		for name, value := range set.Timeslices[0].Values {

			if v, ok := value.(float64); ok {

				ch <- Metric{
					App:   app,
					Name:  name,
					Value: v,
					Label: set.Name,
				}

			}
		}

	}
}

type Exporter struct {
	mu              sync.Mutex
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	metrics         map[string]prometheus.GaugeVec
	api             newRelicApi
}

func NewExporter() *Exporter {
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
	}
}

func (e *Exporter) scrape(ch chan<- Metric) {

	e.error.Set(0)
	e.totalScrapes.Inc()

	now := time.Now().UnixNano()
	log.Debugf("Starting new scrape at %d.", now)

	var apps AppList
	err := apps.get(e.api)
	if err != nil {
		e.error.Set(1)
	}

	apps.sendMetrics(ch)

	for _, app := range apps.Applications {

		var names MetricNames

		err = names.get(e.api, app.Id)
		if err != nil {
			e.error.Set(1)
		}

		var data MetricData

		err = data.get(e.api, app.Id, names)
		if err != nil {
			e.error.Set(1)
		}

		data.sendMetrics(ch, app.Name)

	}

	close(ch)
	e.duration.Set(float64(time.Now().UnixNano()-now) / 1000000000)
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

	e.api.to = time.Now().UTC()
	e.api.from = e.api.to.Add(-time.Duration(e.api.period) * time.Second)

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

type newRelicApi struct {
	server url.URL
	apiKey string
	from   time.Time
	to     time.Time
	period int
}

func NewNewRelicApi(server string, apikey string) *newRelicApi {
	parsed, err := url.Parse(server)
	if err != nil {
		log.Fatal("Could not parse API URL: ", err)
	}
	if apikey == "" {
		log.Fatal("Cannot continue without an API key.")
	}
	return &newRelicApi{
		server: *parsed,
		apiKey: apikey,
	}
}

func (a *newRelicApi) req(path string, params string) ([]byte, error) {

	u := a.server
	u.Path = path
	u.RawQuery = params

	log.Debug("Making API call: ", u.String())

	req := &http.Request{
		Method: "GET",
		URL:    &u,
		Header: http.Header{
			"User-Agent": {UserAgent},
			"X-Api-Key":  {a.apiKey},
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: TlsIgnore,
			},
		},
	}

	var data []byte
	pageCount := 1

	for page := 1; page <= pageCount; page++ {
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Bad response code: %s", resp.Status)
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()
		data = append(data, body...)

		link := resp.Header.Get("Link")
		vals := LinkRexp.FindStringSubmatch(link)

		if len(vals) == 3 { // Full string plus two sub-expressions
			u, err := url.Parse(vals[2]) // Parse the second URL
			if err != nil {
				return nil, err
			}
			pageCount, err = strconv.Atoi(u.Query().Get("page"))
			if err != nil {
				return nil, err
			}
		}
		qry := req.URL.Query()
		qry.Set("page", strconv.Itoa(page+1))
		req.URL.RawQuery = qry.Encode()
	}

	return data, nil

}

func main() {
	var server, apikey, listenAddress, metricPath string
	var period int

	flag.StringVar(&apikey, "api.key", "", "NewRelic API key")
	flag.StringVar(&server, "api.server", "https://api.newrelic.com", "NewRelic API URL")
	flag.IntVar(&period, "api.period", 60, "Period of data to extract in seconds")

	flag.StringVar(&listenAddress, "web.listen-address", ":9126", "Address to listen on for web interface and telemetry.")
	flag.StringVar(&metricPath, "web.telemetry-path", "/metrics", "Path under which to expose metrics.")

	flag.Parse()

	api := NewNewRelicApi(server, apikey)
	api.period = period
	exporter := NewExporter()
	exporter.api = *api

	prometheus.MustRegister(exporter)

	http.Handle(metricPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><title>NewRelic exporter</title></head>
<body>
<h1>NewRelic exporter</h1>
<p><a href='` + metricPath + `'>Metrics</a></p>
</body>
</html>
`))
	})

	log.Printf("Listening on %s.", listenAddress)
	err := http.ListenAndServe(listenAddress, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("HTTP server stopped.")
}

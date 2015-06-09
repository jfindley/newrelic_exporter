package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Header to set for API auth
const APIHDR string = "X-Api-Key"

// Chunk size of metric requests
const CHUNKSIZE = 10

// This is to support skipping verification for testing and
// is deliberately not exposed to the user
var TLSIGNORE bool = false

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

	body, err := api.req("/v2/applications.json", "")
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, a)
	return err

}

func (a *AppList) Metrics(ch chan<- prometheus.Metric) {

}

type MetricNames struct {
	Metrics []struct {
		Name   string
		Values []string
	}
}

func (m *MetricNames) get(api newRelicApi, appId int) error {

	path := fmt.Sprintf("/v2/applications/%s/metrics.json", strconv.Itoa(appId))

	body, err := api.req(path, "")
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, m)
	return err

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

	// Because the Go client does not yet support 100-continue
	// ( see issue #3665 ),
	// we have to process this in chunks, to ensure the response
	// fits within a single request.

	for i := 0; i < len(nameList); i += CHUNKSIZE {

		var thisData MetricData
		var thisList []string

		if i+CHUNKSIZE > len(nameList) {
			thisList = nameList[i:]
		} else {
			thisList = nameList[i : i+CHUNKSIZE]
		}

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
			return err
		}

		// We ignore unmarshal errors
		json.Unmarshal(body, &thisData)

		allData := m.Metric_Data.Metrics
		allData = append(allData, thisData.Metric_Data.Metrics...)
		m.Metric_Data.Metrics = allData

	}

	return nil
}

type newRelicApi struct {
	server string
	apiKey string
	from   time.Time
	to     time.Time
	period int
}

func (a *newRelicApi) req(path string, data string) ([]byte, error) {

	u := fmt.Sprintf("%s%s?%s", a.server, path, data)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: TLSIGNORE,
			},
		},
	}

	req.Header.Set(APIHDR, a.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Bad response code: %s", resp.Status)
	}

	return body, nil

}

func FindMetrics(api newRelicApi, ch chan<- prometheus.Metric) {

	var apps AppList
	err := apps.get(api)
	if err != nil {
		fmt.Println(err)
	}

	// hand the app-wide metrics off to a parsing function here
	// fmt.Println(apps)

	for _, app := range apps.Applications {

		var names MetricNames

		err = names.get(api, app.Id)
		if err != nil {
			fmt.Println(err)
		}

		// fmt.Println(names)

		var data MetricData

		err = data.get(api, app.Id, names)
		if err != nil {
			fmt.Println(err)
		}

		// fmt.Println(data)

	}

	close(ch)

}

func main() {

	var api newRelicApi

	flag.StringVar(&api.apiKey, "api.key", "", "NewRelic API key")
	flag.IntVar(&api.period, "api.period", 60, "Period of data to extract in seconds")

	// listenAddress = flag.String("web.listen-address", ":9104", "Address to listen on for web interface and telemetry.")
	// metricPath = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")

	flag.Parse()

	api.to = time.Now().UTC()
	api.from = api.to.Add(-time.Duration(api.period) * time.Second)

	api.server = "https://api.newrelic.com"

	ch := make(chan prometheus.Metric)

	FindMetrics(api, ch)

}

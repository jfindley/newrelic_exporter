package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testApiKey string = "205071e37e95bdaa327c62ccd3201da9289ccd17"
var testApiAppId int = 9045822
var testTimeout time.Duration = 5 * time.Second
var testPostData string = "names[]=Datastore%2Fstatement%2FJDBC%2Fmessages%2Finsert&raw=true&summarize=true&period=0&from=0001-01-01T00:00:00Z&to=0001-01-01T00:00:00Z"

func TestAppListGet(t *testing.T) {

	ts, err := testServer()
	if err != nil {
		t.Fatal(err)
	}

	defer ts.Close()

	api := NewNewRelicAPI(ts.URL, testApiKey, testTimeout)
	api.client = &http.Client{
		Timeout: testTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	var app AppList

	err = app.get(api)
	if err != nil {
		t.Fatal(err)
	}

	if len(app.Applications) != 1 {
		t.Fatal("Expected 1 application, got", len(app.Applications))
	}

	a := app.Applications[0]

	switch {

	case a.ID != testApiAppId:
		t.Fatal("Wrong ID")

	case a.Health != "green":
		t.Fatal("Wrong health status")

	case a.Name != "Test/Client/Name":
		t.Fatal("Wrong name")

	case a.AppSummary["throughput"] != 54.7:
		t.Fatal("Wrong throughput")

	case a.AppSummary["host_count"] != 3:
		t.Fatal("Wrong host count")

	case a.UsrSummary["response_time"] != 4.61:
		t.Fatal("Wrong response time")

	}

}

func TestMetricNamesGet(t *testing.T) {

	ts, err := testServer()
	if err != nil {
		t.Fatal(err)
	}

	defer ts.Close()

	api := NewNewRelicAPI(ts.URL, testApiKey, testTimeout)
	api.client = &http.Client{
		Timeout: testTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	var names MetricNames

	err = names.get(api, testApiAppId)
	if err != nil {
		t.Fatal(err)
	}

	if len(names.Metrics) != 2 {
		t.Fatal("Expected 2 name sets, got", len(names.Metrics))
	}

	if len(names.Metrics[0].Values) != 10 {
		t.Fatal("Expected 10 metric names")
	}

	if names.Metrics[0].Name != "Datastore/statement/JDBC/messages/insert" {
		t.Fatal("Wrong application name")
	}
	if names.Metrics[1].Name != "Datastore/statement/JDBC/messages/update" {
		t.Fatal("Wrong application name")
	}

}

func TestMetricValuesGet(t *testing.T) {

	ts, err := testServer()
	if err != nil {
		t.Fatal(err)
	}

	defer ts.Close()

	api := NewNewRelicAPI(ts.URL, testApiKey, testTimeout)
	api.client = &http.Client{
		Timeout: testTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	var data MetricData
	var names MetricNames

	err = names.get(api, testApiAppId)
	if err != nil {
		t.Fatal(err)
	}

	err = data.get(api, testApiAppId, names)
	if err != nil {
		t.Fatal(err)
	}

	if len(data.Metric_Data.Metrics) != 1 {
		t.Fatal("Expected 1 metric sets")
	}

	if len(data.Metric_Data.Metrics[0].Timeslices) != 1 {
		t.Fatal("Expected 1 timeslice")
	}

	appData := data.Metric_Data.Metrics[0].Timeslices[0]

	if len(appData.Values) != 10 {
		t.Fatal("Expected 10 data points")
	}

	if appData.Values["call_count"].(float64) != 2 {
		t.Fatal("Wrong call_count value")
	}

	if appData.Values["calls_per_minute"].(float64) != 2.03 {
		t.Fatal("Wrong calls_per_minute value")
	}

}

func TestScrapeAPI(t *testing.T) {

	ts, err := testServer()
	if err != nil {
		t.Fatal(err)
	}

	defer ts.Close()

	exporter := NewExporter()
	exporter.api = NewNewRelicAPI(ts.URL, testApiKey, testTimeout)
	exporter.api.client = &http.Client{
		Timeout: testTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	var recieved []Metric

	metrics := make(chan Metric)

	go exporter.scrape(metrics)

	for m := range metrics {
		recieved = append(recieved, m)
	}

	if len(recieved) != 21 {
		t.Fatal("Expected 21 metrics")
	}

}

func testServer() (ts *httptest.Server, err error) {

	ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Header.Get("X-Api-Key") != testApiKey {
			w.WriteHeader(403)
		}

		var body []byte
		var sourceFile string

		firstLink := fmt.Sprintf(
			"<%s%s?page=%d>; rel=%s, <%s%s?page=%d>; rel=%s",
			ts.URL, r.URL.Path, 2, `"next"`,
			ts.URL, r.URL.Path, 2, `"last"`)

		secondLink := fmt.Sprintf(
			"<%s%s?page=%d>; rel=%s, <%s%s?page=%d>; rel=%s",
			ts.URL, r.URL.Path, 1, `"first"`,
			ts.URL, r.URL.Path, 1, `"prev"`)

		switch r.URL.Path {

		case "/v2/applications.json":
			sourceFile = "_testing/application_list.json"

		case "/v2/applications/9045822/metrics.json":
			if r.URL.Query().Get("page") == "2" {
				sourceFile = ("_testing/metric_names_2.json")
				w.Header().Set("Link", secondLink)
			} else {
				sourceFile = ("_testing/metric_names.json")
				w.Header().Set("Link", firstLink)
			}

		case "/v2/applications/9045822/metrics/data.json":
			sourceFile = ("_testing/metric_data.json")

		default:
			w.WriteHeader(404)
			return

		}

		body, err = ioutil.ReadFile(sourceFile)
		if err != nil {
			return
		}

		w.WriteHeader(200)
		w.Write(body)

	}))

	return
}

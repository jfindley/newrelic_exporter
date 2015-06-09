package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	// "github.com/prometheus/client_golang/prometheus"
)

// Header to set for API auth
const APIHDR string = "X-Api-Key"

// This is to support skipping verification for testing and
// is deliberately not exposed to the user
var TLSIGNORE bool = false

type AppList struct {
	Applications []struct {
		Id         int
		Name       string
		Health     string      `json:"health_status"`
		AppSummary AppStats    `json:"application_summary"`
		UsrSummary AppUsrStats `json:"end_user_summary"`
	}
}

type AppStats struct {
	ResponseTime            float64 `json:"response_time"`
	Throughput              float64 `json:"throughput"`
	ErrorRate               float64 `json:"error_rate"`
	ApdexTarget             float64 `json:"apdex_target"`
	ApdexScore              float64 `json:"apdex_score"`
	HostCount               int     `json:"host_count"`
	InstanceCount           int     `json:"instance_count"`
	ConcurrentInstanceCount int     `json:"concurrent_instance_count"`
}

type AppUsrStats struct {
	ResponseTime float64 `json:"response_time"`
	Throughput   float64 `json:"throughput"`
	ErrorRate    float64 `json:"error_rate"`
	ApdexTarget  float64 `json:"apdex_target"`
	ApdexScore   float64 `json:"apdex_score"`
}

func (a *AppList) get(api newRelicApi) error {

	body, err := api.req("/v2/applications.json", "")
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, a)
	return err

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
			Timeslices []struct {
				Values map[string]float64
			}
		}
	}
}

func (m *MetricData) get(api newRelicApi, appId int, values MetricNames) error {

	if len(values.Metrics) != 1 {
		return errors.New("Only one set of metric names can be processed at a time")
	}

	path := fmt.Sprintf("/v2/applications/%s/metrics/data.json", strconv.Itoa(appId))

	data := fmt.Sprintf(
		"names[]=%s&values[]=%s&summarize=true",
		strings.Join(values.Metrics[0].Values, "&values[]="))

	body, err := api.req(path, data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, m)
	return err
}

type newRelicApi struct {
	server string
	apikey string
}

func (a *newRelicApi) req(path string, data string) ([]byte, error) {

	req, err := http.NewRequest("GET", a.server+path, bytes.NewBuffer([]byte(data)))
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

	req.Header.Set(APIHDR, a.apikey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Bad response code: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)

	return body, err

}

func main() {

}

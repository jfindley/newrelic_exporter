package main

import (
// "encoding/json"
// "net/http"

// "github.com/prometheus/client_golang/prometheus"
)

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

type MetricNames struct {
	Metrics []struct {
		Name   string
		Values []string
	}
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

func main() {

}

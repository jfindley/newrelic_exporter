package main

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

// Test the data structures parse sample JSON input correctly
func TestJsonParsing(t *testing.T) {
	appList, err := ioutil.ReadFile("_testing/application_list.json")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ioutil.ReadFile("_testing/metric_data.json")
	if err != nil {
		t.Fatal(err)
	}

	names, err := ioutil.ReadFile("_testing/metric_names.json")
	if err != nil {
		t.Fatal(err)
	}

	var (
		parsedApp   AppList
		parsedNames MetricNames
		parsedData  MetricData
	)

	err = json.Unmarshal(appList, &parsedApp)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(data, &parsedData)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(names, &parsedNames)
	if err != nil {
		t.Fatal(err)
	}

	if len(parsedApp.Applications) != 1 {
		t.Fatal("Expected 1 application")
	}

	app := parsedApp.Applications[0]

	switch {

	case app.Id != 9045822:
		t.Fatal("Wrong ID")

	case app.Health != "green":
		t.Fatal("Wrong health status")

	case app.Name != "Test/Client/Name":
		t.Fatal("Wrong name")

	case app.AppSummary.Throughput != 54.7:
		t.Fatal("Wrong throughput")

	case app.UsrSummary.ResponseTime != 4.61:
		t.Fatal("Wrong response time")

	}

	if len(parsedData.Metric_Data.Metrics) != 1 {
		t.Fatal("Expected 1 metric set")
	}

	if len(parsedData.Metric_Data.Metrics[0].Timeslices) != 1 {
		t.Fatal("Expected 1 timeslice")
	}

	appData := parsedData.Metric_Data.Metrics[0].Timeslices[0]

	if len(appData.Values) != 10 {
		t.Fatal("Expected 10 data points")
	}

	if appData.Values["call_count"] != 2 {
		t.Fatal("Wrong call_count value")
	}

	if appData.Values["calls_per_minute"] != 2.03 {
		t.Fatal("Wrong calls_per_minute value")
	}
}

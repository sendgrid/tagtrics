package tagtrics

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/rcrowley/go-metrics"
)

type subMetrics struct {
	Counter metrics.Counter
}

type testMetrics struct {
	SubItem struct {
		Counter   metrics.Counter   `metric:"counter"`
		Timer     metrics.Timer     `metric:"timer"`
		Meter     metrics.Meter     `metric:"meter"`
		Gauge     metrics.Gauge     // Metric should be derived from name
		Histogram metrics.Histogram `metric:"histogram"`
	} `metric:"subitem"`
	Counter   metrics.Counter   `metric:"counter"`
	Timer     metrics.Timer     `metric:"timer"`
	Meter     metrics.Meter     `metric:"meter"`
	Gauge     metrics.Gauge     `metric:"gauge"`
	Histogram metrics.Histogram `metric:"histogram"`
	Map       map[string]*subMetrics
}

type validateMetricsData struct {
	SubCounter       map[string]float64 `json:"subitem_counter"`
	SubTimer         map[string]float64 `json:"subitem_timer"`
	SubMeter         map[string]float64 `json:"subitem_meter"`
	SubGauge         map[string]float64 `json:"subitem_gauge"`
	SubHistogram     map[string]float64 `json:"subitem_histogram"`
	Counter          map[string]float64 `json:"counter"`
	Timer            map[string]float64 `json:"timer"`
	Meter            map[string]float64 `json:"meter"`
	Gauge            map[string]float64 `json:"gauge"`
	Histogram        map[string]float64 `json:"histogram"`
	MapThing1Counter map[string]float64 `json:"map_thing1_counter"`
	MapThing2Counter map[string]float64 `json:"map_thing2_counter"`
}

func TestMetricTags(t *testing.T) {
	m := &testMetrics{Map: map[string]*subMetrics{
		"thing1": &subMetrics{},
		"thing2": &subMetrics{},
	}}

	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	h := func() {
		once.Do(wg.Done)
	}
	updateInterval := 100 * time.Millisecond
	mTags := NewMetricTags(m, h, updateInterval, metrics.DefaultRegistry, "_")
	mTags.StatsGCCollection = updateInterval
	mTags.StatsMemCollection = updateInterval
	go mTags.Run()

	// Modify all metrics
	m.SubItem.Counter.Inc(1)
	m.SubItem.Timer.Update(time.Millisecond)
	m.SubItem.Meter.Mark(1)
	m.SubItem.Gauge.Update(1)
	m.SubItem.Histogram.Update(1)
	m.Counter.Inc(1)
	m.Timer.Update(time.Millisecond)
	m.Meter.Mark(1)
	m.Gauge.Update(1)
	m.Histogram.Update(1)
	m.Map["thing1"].Counter.Inc(1)
	m.Map["thing2"].Counter.Inc(2)
	// Make sure our update function gets called at least once
	wg.Wait()
	// Verify all data
	var j validateMetricsData
	err := json.Unmarshal(mTags.ToJSON(), &j)
	if err != nil {
		t.Fatalf("failed to convert metrics data to JSON: %v", err)
	}
	if j.SubCounter["count"] != 1 {
		t.Fatalf("failed to verify data: %v", j.SubCounter)
	}
	if j.SubTimer["count"] != 1 || j.SubTimer["max"] != 1000000 {
		t.Fatalf("failed to verify data: %v", j.SubTimer)
	}
	if j.SubMeter["count"] != 1 || j.SubMeter["mean.rate"] <= 0 {
		t.Fatalf("failed to verify data: %v", j.SubMeter)
	}
	if j.SubGauge["value"] != 1 {
		t.Fatalf("failed to verify data: %v", j.SubGauge)
	}
	if j.SubHistogram["count"] != 1 || j.SubHistogram["99.9%"] != 1 {
		t.Fatalf("failed to verify data: %v", j.SubHistogram)
	}
	if j.Counter["count"] != 1 {
		t.Fatalf("failed to verify data: %v", j.Counter)
	}
	if j.Timer["count"] != 1 || j.Timer["max"] != 1000000 {
		t.Fatalf("failed to verify data: %v", j.Timer)
	}
	if j.Meter["count"] != 1 || j.Meter["mean.rate"] <= 0 {
		t.Fatalf("failed to verify data: %v", j.Meter)
	}
	if j.Gauge["value"] != 1 {
		t.Fatalf("failed to verify data: %v", j.Gauge)
	}
	if j.Histogram["count"] != 1 || j.Histogram["99.9%"] != 1 {
		t.Fatalf("failed to verify data: %v", j.Histogram)
	}
	if j.MapThing1Counter["count"] != 1 {
		t.Fatalf("failed to verify data: %v", j.MapThing1Counter)
	}
	if j.MapThing2Counter["count"] != 2 {
		t.Fatalf("failed to verify data: %v", j.MapThing2Counter)
	}
	mTags.Stop()
}

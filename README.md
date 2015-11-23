# tagtrics
tagtrics allows developers to keep all their metrics in a central `struct` using types from the great library [go-metrics](https://github.com/rcrowley/go-metrics) and initialized with struct tags.  This allows developers to share the instance of this `struct` in the application and easiliy update metrics.  The advantages to this approach include:

* **Logic** - The logic of the application does not change when a new system is used as a centralized metrics store.
* **Typed Metrics** - Metrics do not have to be defined in strings.  Each metric has a type and they can be embedded in structs.  The name of each metric is derived from the structs.
* **JSON** - All the metrics can be represented as JSON and easily exposed over an API to expose real-time stats or generate alerts.

tagtrics also gathers metrics automatically for the Go runtime.  If a tag for a field is not found, the name of metric is derived from the lower case field name.

# Example

```go
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/rcrowley/go-metrics"
	"github.com/sendgrid/tagtrics"
)

// appMetrics holds all metrics for this application.
type appMetrics struct {
	Messages struct {
		Size metrics.Histogram `metric:"size"`
	} `metric:"messages"`
	Connections struct {
		HTTP struct {
			Concurrent metrics.Counter `metric:"concurrent"`
			Count      metrics.Meter   // metric="count", derived from field name
			Errors     metrics.Meter   `metric:"errors"`
			Duration   metrics.Timer   `metric:"duration"`
		} `metric:"http"`
	} `metric:"connections"`
}

// getUpdateGraphiteHandler returns a handler that flushes existing stats in
// registry to Graphite on graphiteHost using namespace.
func getUpdateGraphiteHandler(graphiteHost, namespace string, registry metrics.Registry) (tagtrics.MetricsUpdateHandler, error) {
	// Check that graphiteHost is valid.
	addr, err := net.ResolveTCPAddr("tcp", graphiteHost)
	if err != nil {
		return nil, err
	}
	// Configure Graphite
	c := graphite.GraphiteConfig{
		Addr:         addr,
		Registry:     registry,
		DurationUnit: time.Nanosecond,
		Prefix:       namespace + ".",
		Percentiles:  []float64{0.5, 0.75, 0.95, 0.99, 0.999},
	}
	// Once Graphite is configured return the actual function that will do
	// the metric updates.
	return func() {
		fmt.Println("updating graphite")
		if err := graphite.GraphiteOnce(c); err != nil {
			fmt.Println("error updating graphite", err)
		}
	}, nil
}

// app is a simple HTTP app that will update metrics on each request
type app struct {
	m          *appMetrics
	metricTags *tagtrics.MetricTags
}

// ServeHTTP is the function that will update all metrics.  It handles all
// URL requests.
func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	// Assume this connection is active
	a.m.Connections.HTTP.Concurrent.Inc(1)
	// Keep track of our connection rate
	a.m.Connections.HTTP.Count.Mark(1)
	// The size is the length of the requested URL.  Multiple URLs that vary in
	// length can be sent to test.
	a.m.Messages.Size.Update(int64(len(r.RequestURI)))
	// Return JSON stats.
	w.Header().Set("Content-Type", "application/json")
	w.Write(a.metricTags.ToJSON())
	// Update duration
	a.m.Connections.HTTP.Duration.UpdateSince(startTime)
	// Assume this connection is no longer active
	a.m.Connections.HTTP.Concurrent.Dec(1)
}

func main() {
	// Use the default registry in go-metrics
	reg := metrics.DefaultRegistry
	m := &appMetrics{}
	handler, err := getUpdateGraphiteHandler("localhost:1234", "dev.app.localhost", reg)
	if err != nil {
		log.Fatalf("error creating graphite handler: %v", err)
	}
	// Update Graphite every three seconds
	flushInterval := 3 * time.Second
	metricTags := tagtrics.NewMetricTags(m, handler, flushInterval, reg, ".")

	// You never want to update this often in production but here we update
	// them often to see how they change.
	metricTags.StatsGCCollection = 3 * time.Second
	metricTags.StatsMemCollection = 3 * time.Second

	// Update graphite periodically in the background
	go metricTags.Run()
	myApp := app{m: m, metricTags: metricTags}
	http.ListenAndServe("127.0.0.1:7890", &myApp)
	metricTags.Stop()
}

```

Once running, the stats should change after each request:

```bash
curl -s 'http://127.0.0.1:7890/metrics'|python -mjson.tool
```
```json
{
    "connections.http.concurrent": {
        "count": 1
    },
    "connections.http.count": {
        "15m.rate": 0.0010025873653552021,
        "1m.rate": 0.003568100513704112,
        "5m.rate": 0.0024489296340649874,
        "count": 2,
        "mean.rate": 0.018608122671653474
    },
    "connections.http.duration": {
        "15m.rate": 0.0010025873653552021,
        "1m.rate": 0.003568100513704112,
        "5m.rate": 0.0024489296340649874,
        "75%": 518146,
        "95%": 518146,
        "99%": 518146,
        "99.9%": 518146,
        "count": 1,
        "max": 518146,
        "mean": 518146,
        "mean.rate": 0.009523517940083949,
        "median": 518146,
        "min": 518146,
        "stddev": 0
    },
    "connections.http.errors": {
        "15m.rate": 0,
        "1m.rate": 0,
        "5m.rate": 0,
        "count": 0,
        "mean.rate": 0
    },
    "debug.GCStats.LastGC": {
        "value": 0
    },
    "debug.GCStats.NumGC": {
        "value": 0
    },
    "debug.GCStats.Pause": {
        "75%": 0,
        "95%": 0,
        "99%": 0,
        "99.9%": 0,
        "count": 0,
        "max": 0,
        "mean": 0,
        "median": 0,
        "min": 0,
        "stddev": 0
    },
    "debug.GCStats.PauseTotal": {
        "value": 0
    },
    "debug.ReadGCStats": {
        "15m.rate": 0.2136500992870312,
        "1m.rate": 0.30249466074798775,
        "5m.rate": 0.23667725665187206,
        "75%": 2633,
        "95%": 11992.199999999804,
        "99%": 48789,
        "99.9%": 48789,
        "count": 35,
        "max": 48789,
        "mean": 3700.5714285714284,
        "mean.rate": 0.3328810264894881,
        "median": 2408,
        "min": 1839,
        "stddev": 7736.156920722383
    },
    "messages.size": {
        "75%": 8,
        "95%": 8,
        "99%": 8,
        "99.9%": 8,
        "count": 2,
        "max": 8,
        "mean": 8,
        "median": 8,
        "min": 8,
        "stddev": 0
    },
    "runtime.MemStats.Alloc": {
        "value": 374488
    },
    "runtime.MemStats.BuckHashSys": {
        "value": 1443248
    },
    "runtime.MemStats.DebugGC": {
        "value": 0
    },
    "runtime.MemStats.EnableGC": {
        "value": 1
    },
    "runtime.MemStats.Frees": {
        "value": 0
    },
    "runtime.MemStats.HeapAlloc": {
        "value": 374488
    },
    "runtime.MemStats.HeapIdle": {
        "value": 712704
    },
    "runtime.MemStats.HeapInuse": {
        "value": 892928
    },
    "runtime.MemStats.HeapObjects": {
        "value": 2321
    },
    "runtime.MemStats.HeapReleased": {
        "value": 0
    },
    "runtime.MemStats.HeapSys": {
        "value": 1605632
    },
    "runtime.MemStats.LastGC": {
        "value": 0
    },
    "runtime.MemStats.Lookups": {
        "value": 1
    },
    "runtime.MemStats.MCacheInuse": {
        "value": 9664
    },
    "runtime.MemStats.MCacheSys": {
        "value": 16384
    },
    "runtime.MemStats.MSpanInuse": {
        "value": 9072
    },
    "runtime.MemStats.MSpanSys": {
        "value": 16384
    },
    "runtime.MemStats.Mallocs": {
        "value": 23
    },
    "runtime.MemStats.NextGC": {
        "value": 4194304
    },
    "runtime.MemStats.NumGC": {
        "value": 0
    },
    "runtime.MemStats.PauseNs": {
        "75%": 0,
        "95%": 0,
        "99%": 0,
        "99.9%": 0,
        "count": 0,
        "max": 0,
        "mean": 0,
        "median": 0,
        "min": 0,
        "stddev": 0
    },
    "runtime.MemStats.PauseTotalNs": {
        "value": 0
    },
    "runtime.MemStats.StackInuse": {
        "value": 491520
    },
    "runtime.MemStats.StackSys": {
        "value": 491520
    },
    "runtime.MemStats.Sys": {
        "value": 4458744
    },
    "runtime.MemStats.TotalAlloc": {
        "value": 374488
    },
    "runtime.NumCgoCall": {
        "value": 0
    },
    "runtime.NumGoroutine": {
        "value": 8
    },
    "runtime.ReadMemStats": {
        "15m.rate": 0.2136500992870312,
        "1m.rate": 0.30249466074798775,
        "5m.rate": 0.23667725665187206,
        "75%": 25481,
        "95%": 75027.59999999999,
        "99%": 77946,
        "99.9%": 77946,
        "count": 35,
        "max": 77946,
        "mean": 28313.314285714285,
        "mean.rate": 0.3328811190061803,
        "median": 20574,
        "min": 15176,
        "stddev": 18583.187599504217
    }
}
```

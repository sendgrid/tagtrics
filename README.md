# tagtrics
tagtrics allows developers to keep all their metrics in a central `struct` using types from the great library [go-metrics](https://github.com/rcrowley/go-metrics) and initialized with struct tags.  This allows developers to share the instance of this `struct` in the application and easiliy update metrics.  The advantages to this approach include:

* **Logic** - The logic of the application does not change when a new system is used as a centralized metrics store.
* **Typed Metrics** - Metrics do not have to be defined in strings.  Each metric has a type and they can be embedded in structs.  The name of each metric is derived from the structs.
* **JSON** - All the metrics can be represented as JSON and easily exposed over an API to provide expose real-time stats or generate alerts.


# Example

```
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
		Http struct {
			Concurrent metrics.Counter `metric:"concurrent"`
			Count      metrics.Meter   `metric:"count"`
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
	a.m.Connections.Http.Concurrent.Inc(1)
	// Keep track of our connection rate
	a.m.Connections.Http.Count.Mark(1)
	// The size is the length of the requested URL.  Multiple URLs that vary in
	// length can be sent to test.
	a.m.Messages.Size.Update(int64(len(r.RequestURI)))
	// Return JSON stats.
	w.Header().Set("Content-Type", "application/json")
	w.Write(a.metricTags.ToJSON())
	// Update duration
	a.m.Connections.Http.Duration.UpdateSince(startTime)
	// Assume this connection is no longer active
	a.m.Connections.Http.Concurrent.Dec(1)
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
	updateInterval := time.Duration(3) * time.Second
	metricTags := tagtrics.NewMetricTags(m, handler, updateInterval, reg, ".")
	// You never want to do this in production but here we update these stats
	// often to see how they change.
	metricTags.StatsGCCollection = updateInterval
	metricTags.StatsMemCollection = updateInterval
	// Update graphite periodically in the background
	go metricTags.Run()
	myApp := app{m: m, metricTags: metricTags}
	http.ListenAndServe("127.0.0.1:7890", &myApp)
	metricTags.Stop()
}
```
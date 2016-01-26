package tagtrics

import (
	"bytes"
	"reflect"
	"strings"
	"time"

	metrics "github.com/rcrowley/go-metrics"
)

const (
	// DefaultStatsMemCollection determines how often we sample runtime stats.
	// This stops the world for approximately 200us so don't do it too often.
	DefaultStatsMemCollection = time.Duration(5 * time.Minute)
	// DefaultStatsGCCollection gets the garbage collection stats and is not as
	// expensive as stopping the world, but it isn't cheap so don't do it too
	// often either.
	DefaultStatsGCCollection = time.Duration(1 * time.Minute)
)

// MetricsUpdateHandler is the handler that will be called every
// MetricTags.flushInterval to update the stats remotely.
type MetricsUpdateHandler func()

// MetricTags traverses a given struct to initialize its metrics data types
// for a given namespace so they can be ready to use in the application and
// constantly update a configured source.
type MetricTags struct {
	// quitCh is a channel used to signal that any background goroutine
	// related to this struct should quit.
	quitCh chan struct{}
	// nowHandler is used to overwrite the existing time returned during
	// testing.
	nowHandler func() time.Time
	// metricsData is the struct that holds all metrics data and "metric" tags.
	metricsData interface{}
	// updateHandler is the handler that is called to constantly update stats
	// with a remote system.
	updateHandler MetricsUpdateHandler
	// flushInterval holds how often updateHandler is called.
	flushInterval time.Duration
	// registry is the metrics registry used to initialize all metrics in
	// metricsData as well as the Go runtime metrics.
	registry metrics.Registry
	// StatsMemCollection is how often a sample of the Go runtime memory
	// statistics is collected.  If not set, DefaultStatsMemCollection is used.
	StatsMemCollection time.Duration
	// StatsGCCollection is how often a sample of the Go runtime GC
	// statistics is collected.  If not set, DefaultStatsGCCollection is used.
	StatsGCCollection time.Duration
	// Separator is the separator used in between metric field names while
	// traversing metricsData.  The resulting name is the name assigned to that
	// field.
	separator string
}

// NewMetricTags creates a new MetricTags.  metricsData is the struct containing
// "metric" tags and fields to be initialized in the registry namespace
// separated by separator.  updateHandler is the handler what is called every
// flushInterval to constantly update metrics.  metricsData gets initialized
// before return.
func NewMetricTags(metricsData interface{}, updateHandler MetricsUpdateHandler, flushInterval time.Duration, registry metrics.Registry, separator string) *MetricTags {
	m := &MetricTags{
		quitCh:             make(chan struct{}),
		nowHandler:         time.Now,
		metricsData:        metricsData,
		updateHandler:      updateHandler,
		flushInterval:      flushInterval,
		registry:           registry,
		StatsMemCollection: DefaultStatsMemCollection,
		StatsGCCollection:  DefaultStatsGCCollection,
		separator:          separator,
	}
	// Initialize metric fields
	m.initializeFieldTagPath(reflect.ValueOf(m.metricsData).Elem(), "")
	return m
}

// Run periodically calls m.updateHandler.
func (m *MetricTags) Run() {
	// Collect Go's runtime stats the first time this is run.
	metrics.RegisterDebugGCStats(m.registry)
	metrics.RegisterRuntimeMemStats(m.registry)

	updateTime := m.nowHandler()
	gcTime, memTime := updateTime, updateTime
	for {
		now := m.nowHandler()
		// Get GC runtime stats
		if now.Sub(gcTime) > m.StatsGCCollection {
			metrics.CaptureDebugGCStatsOnce(m.registry)
			gcTime = now
		}
		// Get memory runtime stats
		if now.Sub(memTime) > m.StatsMemCollection {
			metrics.CaptureRuntimeMemStatsOnce(m.registry)
			memTime = now
		}
		select {
		case <-m.quitCh:
			// Update stats one last time
			m.updateHandler()
			m.quitCh <- struct{}{}
			return
		case <-time.After(m.flushInterval):
			m.updateHandler()
		}
	}
}

// Stop stops the Run worker and waits for it to finish.
func (m *MetricTags) Stop() {
	m.quitCh <- struct{}{}
	// Wait for it to quit
	<-m.quitCh
	close(m.quitCh)
}

// initializeFieldTagPath traverses the given struct trying to initialize
// metric values.  The "metric" struct tag is used to determine the name of the
// metrics for each struct field. If there is no "metric" struct tag, the
// lowercased struct field name is used for the metric name. The name is
// prefixed with tags from previous struct fields if any, separated by a dot.
// For example:
//
//     	Messages struct {
//          Smtp struct {
//              Latency metrics.Timer `metric:"latency"`
//          } `metric:"smtp"`
//          Http struct {
//              Latency metrics.Timer `metric:"latency"`
//          } `metric:"http"`
//      } `metric:"messages"`
//
// yields timers with names "messages.smtp.latency" and "messages.http.latency"
// respectively.
//
// If there is no metric tag for a field it is skipped and assumed it is used
// for other purposes such as configuration.
func (m *MetricTags) initializeFieldTagPath(fieldType reflect.Value, prefix string) {
	for i := 0; i < fieldType.NumField(); i++ {
		val := fieldType.Field(i)
		field := fieldType.Type().Field(i)

		tag := field.Tag.Get("metric")
		if tag == "" {
			// If tag isn't found, derive tag from the lower case name of
			// the field.
			tag = strings.ToLower(field.Name)
		}
		if prefix != "" {
			tag = prefix + m.separator + tag
		}

		if field.Type.Kind() == reflect.Struct {
			// Recursively traverse an embedded struct
			m.initializeFieldTagPath(val, tag)
		} else if field.Type.Kind() == reflect.Map && field.Type.Key().Kind() == reflect.String {
			// If this is a map[string]Something, then use the string key as bucket name and recursively generate the metrics below
			for _, k := range val.MapKeys() {
				m.initializeFieldTagPath(val.MapIndex(k).Elem(), tag+m.separator+k.String())
			}
		} else {
			// Found a field, initialize
			switch field.Type.String() {
			case "metrics.Counter":
				c := metrics.NewCounter()
				metrics.Register(tag, c)
				val.Set(reflect.ValueOf(c))
			case "metrics.Timer":
				t := metrics.NewTimer()
				metrics.Register(tag, t)
				val.Set(reflect.ValueOf(t))
			case "metrics.Meter":
				m := metrics.NewMeter()
				metrics.Register(tag, m)
				val.Set(reflect.ValueOf(m))
			case "metrics.Gauge":
				g := metrics.NewGauge()
				metrics.Register(tag, g)
				val.Set(reflect.ValueOf(g))
			case "metrics.Histogram":
				s := metrics.NewUniformSample(1028)
				h := metrics.NewHistogram(s)
				metrics.Register(tag, h)
				val.Set(reflect.ValueOf(h))
			}
		}
	}
}

// ToJSON returns a representation of all the metrics in JSON format.
func (m *MetricTags) ToJSON() []byte {
	buf := bytes.NewBuffer(nil)
	metrics.WriteJSONOnce(m.registry, buf)
	return buf.Bytes()
}

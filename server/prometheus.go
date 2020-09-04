package server

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rcrowley/go-metrics"
	"github.com/siddontang/go-log/log"
	"net/http"
	"strings"
	"time"
)

type PrometheusServer struct {
	addr   string
	server *http.Server
	ctx    context.Context
	cancel context.CancelFunc

	namespace     string
	registry      metrics.Registry
	subsystem     string
	promRegistry  prometheus.Registerer
	flushInterval time.Duration
	tick          *time.Timer
	gauges        map[string]prometheus.Gauge
}

func NewPrometheusServer(addr string, r metrics.Registry, promRegistry prometheus.Registerer, flushInterval time.Duration) *PrometheusServer {
	if len(addr) == 0 {
		return nil
	}

	p := &PrometheusServer{
		addr:          addr,
		namespace:     "porter",
		registry:      r,
		subsystem:     "metrics",
		promRegistry:  promRegistry,
		flushInterval: flushInterval,
		tick:          time.NewTimer(flushInterval),
		gauges:        make(map[string]prometheus.Gauge),
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	p.server = &http.Server{Addr: addr, Handler: mux}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	return p
}

func (ps PrometheusServer) flattenKey(key string) string {
	key = strings.Replace(key, "", "_", -1)
	key = strings.Replace(key, ".", "_", -1)
	key = strings.Replace(key, "_", "_", -1)
	key = strings.Replace(key, "=", "_", -1)
	return key
}

func (ps PrometheusServer) gaugeFromNameAndValue(name string, val float64) {
	key := fmt.Sprintf("%s_%s_%s", ps.namespace, ps.subsystem, name)
	g, ok := ps.gauges[key]
	if !ok {
		g = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: ps.flattenKey(ps.namespace),
			Subsystem: ps.flattenKey(ps.subsystem),
			Name:      ps.flattenKey(name),
			Help:      name,
		})
		ps.promRegistry.MustRegister(g)
		ps.gauges[key] = g
	}
	g.Set(val)
}

//Run prometheus server
func (ps *PrometheusServer) Run() {
	go ps.updatePrometheusMetrics()
	err := ps.server.ListenAndServe()
	if err != nil {
		log.Infof("PrometheusServer ListenAndServe error,err:%s", err)
	}
}

func (ps *PrometheusServer) Stop() {
	ps.cancel()
	ps.tick.Stop()
	ps.server.Shutdown(nil)
}

func (ps PrometheusServer) updatePrometheusMetrics() {
	for true {
		select {
		case <-ps.ctx.Done():
			return
		case <-ps.tick.C:
			log.Infof("update Prometheus Metrics Once")
			ps.updatePrometheusMetricsOnce()
		}
	}
}

func (ps PrometheusServer) updatePrometheusMetricsOnce() error {
	ps.registry.Each(func(name string, i interface{}) {
		switch metric := i.(type) {
		case metrics.Counter:
			ps.gaugeFromNameAndValue(name, float64(metric.Count()))
		case metrics.Gauge:
			ps.gaugeFromNameAndValue(name, float64(metric.Value()))
		case metrics.GaugeFloat64:
			ps.gaugeFromNameAndValue(name, float64(metric.Value()))
		case metrics.Histogram:
			snap := metric.Snapshot()
			ps.gaugeFromNameAndValue(name+".mean", snap.Mean())
			ps.gaugeFromNameAndValue(name+".p95", snap.Percentile(0.95))
			ps.gaugeFromNameAndValue(name+".max", float64(snap.Max()))
		case metrics.Meter:
			lastSample := metric.Snapshot().Rate1()
			ps.gaugeFromNameAndValue(name, float64(lastSample))
		case metrics.Timer:
			lastSample := metric.Snapshot().Rate1()
			ps.gaugeFromNameAndValue(name, float64(lastSample))
		}
	})
	return nil
}

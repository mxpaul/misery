package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/mxpaul/misery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Stat struct {
	SecondsFromStart     *prometheus.CounterVec   `misery:"name=seconds_from_start,labels=[thread],help='seconds since application start'" json:"seconds_from_start"`
	UnusedDefaultCounter *prometheus.CounterVec   `json:"unused_default_counter"`
	RandomDuration       *prometheus.HistogramVec `misery:"labels=[thread],buckets=[0.0001, 0.001, 0.01, 0.1, 0.2, 0.3, 0.5, 1.0, 2.0, 10, 20, 50, 100]" json:"random_duration"`
}

type Application struct {
	Stat     Stat
	Registry *prometheus.Registry
	Router   http.Handler
}

func (a *Application) Init() {
	rand.Seed(time.Now().UnixNano())
	a.Registry = prometheus.NewRegistry()

	mux := http.NewServeMux()
	metricsHandler := promhttp.HandlerFor(a.Registry, promhttp.HandlerOpts{})
	mux.Handle("/metrics", metricsHandler)

	a.Router = mux
}

func (a *Application) Run() {
	go func() {
		for {
			select {
			case <-time.After(1 * time.Second):
				a.Stat.SecondsFromStart.With(prometheus.Labels{"thread": "main"}).Inc()
				go func() {
					start := time.Now()
					// tm := prometheus.NewTimer(a.Stat.RandomDuration.With(prometheus.Labels{"thread": "main"}))
					time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
					// tm.ObserveDuration()
					reqDuration := time.Since(start)
					a.Stat.RandomDuration.With(prometheus.Labels{"thread": "main"}).Observe(reqDuration.Seconds())
					log.Printf("duration: %v", reqDuration.Seconds())
				}()
			}
		}
	}()

	listenAddr := "127.0.0.1:8000"
	log.Printf("Listen at %s", listenAddr)
	log.Printf("use curl -v http://%s/metrics for test", listenAddr)
	if err := http.ListenAndServe(listenAddr, a.Router); err != nil {
		log.Panicf("error while serving metrics: %s", err)
	}
	log.Printf("exiting")
}

func main() {
	app := Application{}
	app.Init()

	if err := misery.RegisterMetrics(&app.Stat, app.Registry); err != nil {
		log.Fatalf("metrics register failed: %v", err)
	}

	app.Run()
}

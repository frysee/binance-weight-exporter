package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/procyon-projects/chrono"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const endpoint = "https://api.binance.com"
const namespace = "binance"
const cheapRequest = "/api/v3/ping"

//Define a struct for you collector that contains pointers
//to prometheus descriptors for each metric you wish to expose.
//Note you can also include fields of other types if they provide utility
//but we just won't be exposing them as metrics.
type WeightCollector struct {
	up               *prometheus.Desc
	weightUsed       *prometheus.Desc
	weightUsedMinute *prometheus.Desc
	binanceEndpoint  string
	lastWeight       float64
	isUp             float64
}

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client        = &http.Client{Transport: tr}
	taskScheduler = chrono.NewDefaultTaskScheduler()

	listenAddress = flag.String("web.listen-address", ":9133",
		"Address to listen on for telemetry")
	metricsPath = flag.String("web.telemetry-path", "/metrics",
		"Path under which to expose metrics")
	autoScrape = flag.Bool("auto-scrape", false, "Path under which to expose metrics")
)

func NewCollector(binanceEndpoint string) *WeightCollector {
	return &WeightCollector{
		binanceEndpoint: binanceEndpoint,
		lastWeight:      0,
		isUp:            0,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Was the last Binance API query successful.",
			nil, nil,
		),
		// Headers `X-MBX-USED-WEIGHT-(intervalNum)(intervalLetter)` will give your current used
		// request weight for the (intervalNum)(intervalLetter) rate limiter.
		// For example, if there is a one minute request rate weight limiter set, you will get a
		// `X-MBX-USED-WEIGHT-1M` header in the response. The legacy header `X-MBX-USED-WEIGHT`
		// will still be returned and will represent the current used weight for the one minute
		// request rate weight limit.
		weightUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "weight_used"),
			"Weight used (MBX-USED-WEIGHT).",
			nil, nil,
		),
		weightUsedMinute: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "weight_used_1m"),
			"Weight used per minmute (MBX-USED-WEIGHT-1M).",
			nil, nil,
		),
	}
}

func (c *WeightCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.weightUsed
	ch <- c.weightUsedMinute
}

func (c *WeightCollector) Collect(ch chan<- prometheus.Metric) {
	if !*autoScrape {
		c.RequestWeight()
	}
	c.UpdateMetrics(ch)
}

func (c *WeightCollector) RequestWeight() {

	now := time.Now()
	log.Println("Weight requested at " + now.String())
	// Load BTC klines to get response where we can parse the header
	req, err := http.NewRequest("GET", c.binanceEndpoint+cheapRequest, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Make request and show output.
	resp, err := client.Do(req)
	if err != nil {
		log.Print(err)
		c.isUp = 0
		return
	}

	c.isUp = 1
	c.lastWeight, _ = strconv.ParseFloat(resp.Header.Get("x-mbx-used-weight-1m"), 64)

	// Discard response
	defer resp.Body.Close()
}

func (c *WeightCollector) UpdateMetrics(ch chan<- prometheus.Metric) {

	if c.isUp == 0 {
		ch <- prometheus.MustNewConstMetric(
			c.up, prometheus.GaugeValue, 0,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		c.up, prometheus.GaugeValue, 1,
	)

	ch <- prometheus.MustNewConstMetric(
		c.weightUsed, prometheus.GaugeValue, c.lastWeight,
	)

	ch <- prometheus.MustNewConstMetric(
		c.weightUsedMinute, prometheus.GaugeValue, c.lastWeight,
	)

	log.Println("Endpoint scraped")
}

func main() {
	flag.Parse()

	exporter := NewCollector(endpoint)
	prometheus.MustRegister(exporter)

	// Check API every minute at second 58
	if *autoScrape {
		now := time.Now()
		_, err := taskScheduler.ScheduleAtFixedRate(func(ctx context.Context) {
			exporter.RequestWeight()
		}, 1*time.Minute, chrono.WithStartTime(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 58))

		if err == nil {
			log.Print("Started schedule.")
		}
	}

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Binance API Weight Exporter</title></head>
             <body>
             <h1>Binance API Weight Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

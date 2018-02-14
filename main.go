package main

import (
	"flag"
	"net/http"

	"github.com/banzaicloud/aws-autoscaling-exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"strings"
)

var (
	addr           = flag.String("listen-address", ":8089", "The address to listen on for HTTP requests.")
	metricsPath    = flag.String("metrics-path", "/metrics", "path to metrics endpoint")
	rawLevel       = flag.String("log-level", "info", "log level")
	region         = flag.String("region", "eu-west-1", "AWS region that the exporter should query")
	groupsFlag     = flag.String("auto-scaling-groups", "", "Comma separated list of auto scaling groups to monitor. Empty value means all groups in the region.")
	recommenderUrl = flag.String("recommender-url", "http://localhost:9090", "URL of the spot instance recommender")
)

func init() {
	flag.Parse()
	parsedLevel, err := log.ParseLevel(*rawLevel)
	if err != nil {
		log.WithError(err).Warnf("Couldn't parse log level, using default: %s", log.GetLevel())
	} else {
		log.SetLevel(parsedLevel)
		log.Debugf("Set log level to %s", parsedLevel)
	}
}

func main() {
	log.Info("Starting AWS Auto Scaling Group exporter")
	log.Infof("Starting metric http endpoint on %s", *addr)

	var groups []string
	if *groupsFlag != "" {
		groups = strings.Split(strings.Replace(*groupsFlag, " ", "", -1), ",")
	}
	exporter, err := exporter.NewExporter(*region, groups, *recommenderUrl)
	if err != nil {
		log.Fatal(err)
	}

	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", rootHandler)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<html>
		<head><title>AWS Auto Scaling Group Exporter</title></head>
		<body>
		<h1>AWS Auto Scaling Group Exporter</h1>
		<p><a href="` + *metricsPath + `">Metrics</a></p>
		</body>
		</html>
	`))

}

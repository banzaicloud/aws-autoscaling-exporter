package exporter

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Exporter implements the prometheus.Exporter interface, and exports Redis metrics.
type Exporter struct {
	session      *session.Session
	duration     prometheus.Gauge
	scrapeErrors prometheus.Gauge
	totalScrapes prometheus.Counter
	metrics      map[string]*prometheus.GaugeVec
	metricsMtx   sync.RWMutex
	sync.RWMutex
}

// NewAutoscalingExporter returns a new exporter of AWS Autoscaling group metrics.
func NewAutoscalingExporter(region string) (*Exporter, error) {

	session, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		log.WithError(err).Error("Error creating AWS session")
		return nil, err
	}

	e := Exporter{
		session: session,
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_autoscaling",
			Name:      "scrape_duration_seconds",
			Help:      "The scrape duration.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "aws_autoscaling",
			Name:      "scrapes_total",
			Help:      "Total AWS autoscaling group scrapes.",
		}),
		scrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_autoscaling",
			Name:      "scrape_error",
			Help:      "The scrape error status.",
		}),
		metrics: map[string]*prometheus.GaugeVec{
			"pending_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "pending_instances_total",
				Help:      "Total number of pending instances in the auto scaling group",
			}, []string{"asg_name"}),
			"inservice_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "inservice_instances_total",
				Help:      "Total number of in service instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
			"standby_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "standby_instances_total",
				Help:      "Total number of standby instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
			"terminating_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "terminating_instances_total",
				Help:      "Total number of terminating instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
			"unrecommended_spot_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "unrecommended_spot_instances_total",
				Help:      "Total number of unrecommended spot instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
			"spot_instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "spot_instances_total",
				Help:      "Total number of spot instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
			"instances_total": prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      "instances_total",
				Help:      "Total number of instances in the auto scaling group",
			}, []string{"asg_name", "region"}),
		},
	}

	return &e, nil
}

// Describe outputs metric descriptions.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.metrics {
		m.Describe(ch)
	}
	ch <- e.duration.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeErrors.Desc()
}

// Collect fetches info from the AWS API and the BanzaiCloud recommendation API
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	log.Info("collecting metrics...")
}

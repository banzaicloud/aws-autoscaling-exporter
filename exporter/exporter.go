package exporter

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Exporter implements the prometheus.Exporter interface, and exports AWS AutoScaling metrics.
type Exporter struct {
	session      *session.Session
	duration     prometheus.Gauge
	scrapeErrors prometheus.Gauge
	totalScrapes prometheus.Counter
	metrics      map[string]*prometheus.GaugeVec
	metricsMtx   sync.RWMutex
	sync.RWMutex
}

type scrapeResult struct {
	Name             string
	Value            float64
	AutoScalingGroup string
	Region           string
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

	scrapes := make(chan scrapeResult)

	e.Lock()
	defer e.Unlock()

	go e.scrape(scrapes)
	e.setMetrics(scrapes)

	e.duration.Collect(ch)
	e.totalScrapes.Collect(ch)
	e.scrapeErrors.Collect(ch)

	for _, m := range e.metrics {
		m.Collect(ch)
	}
}

func (e *Exporter) scrape(scrapes chan<- scrapeResult) {

	defer close(scrapes)
	now := time.Now().UnixNano()
	e.totalScrapes.Inc()

	var errorCount uint64 = 0

	asgSvc := autoscaling.New(e.session, aws.NewConfig())

	// TODO: API pagination
	result, err := asgSvc.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{})
	if err != nil {
		log.WithError(err).Error("An error happened while fetching AutoScaling Groups")
		errorCount++
	}
	log.Debug("Number of AutoScaling Groups found:", len(result.AutoScalingGroups))

	var wg sync.WaitGroup
	for _, asg := range result.AutoScalingGroups {
		wg.Add(1)
		go func(asg *autoscaling.Group) {
			defer wg.Done()
			if err := e.scrapeAsg(scrapes, asg); err != nil {
				log.WithField("autoScalingGroup", *asg.AutoScalingGroupName).Error(err)
				atomic.AddUint64(&errorCount, 1)
			}
		}(asg)
	}
	wg.Wait()

	e.scrapeErrors.Set(float64(atomic.LoadUint64(&errorCount)))
	e.duration.Set(float64(time.Now().UnixNano()-now) / 1000000000)
}

func (e *Exporter) setMetrics(scrapes <-chan scrapeResult) {
	for scr := range scrapes {
		name := scr.Name
		if _, ok := e.metrics[name]; !ok {
			e.metricsMtx.Lock()
			e.metrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "aws_autoscaling",
				Name:      name,
			}, []string{"asg_name", "region"})
			e.metricsMtx.Unlock()
		}
		var labels prometheus.Labels = map[string]string{"asg_name": scr.AutoScalingGroup, "region": scr.Region}
		e.metrics[name].With(labels).Set(float64(scr.Value))
	}
}

func (e *Exporter) scrapeAsg(scrapes chan<- scrapeResult, asg *autoscaling.Group) error {
	log.WithField("autoScalingGroup", *asg.AutoScalingGroupName).Debug("getting metrics about ASG")
	scrapes <- scrapeResult{
		Name:             "instances_total",
		Value:            float64(len(asg.Instances)),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}
	return nil
}

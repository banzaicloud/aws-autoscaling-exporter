package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Exporter implements the prometheus.Exporter interface, and exports AWS AutoScaling metrics.
type Exporter struct {
	session        *session.Session
	recommenderUrl string
	duration       prometheus.Gauge
	scrapeErrors   prometheus.Gauge
	totalScrapes   prometheus.Counter
	metrics        map[string]*prometheus.GaugeVec
	metricsMtx     sync.RWMutex
	sync.RWMutex
}

type ScrapeResult struct {
	Name             string
	Value            float64
	AutoScalingGroup string
	Region           string
}

type Recommendation map[string][]InstanceTypeRecommendation

type InstanceTypeRecommendation struct {
	InstanceTypeName   string  `json:"InstanceTypeName"`
	CurrentPrice       string  `json:"CurrentPrice"`
	AvgPriceFor24Hours float32 `json:"AvgPriceFor24Hours"`
	OnDemandPrice      string  `json:"OnDemandPrice"`
	SuggestedBidPrice  string  `json:"SuggestedBidPrice"`
	CostScore          string  `json:"CostScore"`
	StabilityScore     float32 `json:"StabilityScore"`
}

type Instance struct {
	InstanceId       string
	InstanceType     string
	AvailabilityZone string
	SpotBidPrice     string
}

// NewAutoscalingExporter returns a new exporter of AWS Autoscaling group metrics.
func NewAutoscalingExporter(region string, recommenderUrl string) (*Exporter, error) {

	session, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		log.WithError(err).Error("Error creating AWS session")
		return nil, err
	}

	e := Exporter{
		session:        session,
		recommenderUrl: recommenderUrl,
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
			}, []string{"asg_name", "region"}),
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

	scrapes := make(chan ScrapeResult)

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

func (e *Exporter) scrape(scrapes chan<- ScrapeResult) {

	defer close(scrapes)
	now := time.Now().UnixNano()
	e.totalScrapes.Inc()

	var errorCount uint64 = 0

	recommendation, err := e.getRecommendations()
	if err != nil {
		log.WithError(err).Error("Failed to get recommendations, recommendation related metrics will not be reported.")
		atomic.AddUint64(&errorCount, 1)
	}

	asgSvc := autoscaling.New(e.session, aws.NewConfig())
	err = asgSvc.DescribeAutoScalingGroupsPages(&autoscaling.DescribeAutoScalingGroupsInput{}, func(result *autoscaling.DescribeAutoScalingGroupsOutput, lastPage bool) bool {
		log.Debugf("Number of AutoScaling Groups found: %d [lastPage = %t]", len(result.AutoScalingGroups), lastPage)
		var wg sync.WaitGroup
		for _, asg := range result.AutoScalingGroups {
			wg.Add(1)
			go func(asg *autoscaling.Group) {
				defer wg.Done()
				if err := e.scrapeAsg(scrapes, asg, recommendation); err != nil {
					log.WithField("autoScalingGroup", *asg.AutoScalingGroupName).Error(err)
					atomic.AddUint64(&errorCount, 1)
				}
			}(asg)
		}
		wg.Wait()
		return true
	})
	if err != nil {
		log.WithError(err).Error("An error happened while fetching AutoScaling Groups")
		atomic.AddUint64(&errorCount, 1)
	}

	e.scrapeErrors.Set(float64(atomic.LoadUint64(&errorCount)))
	e.duration.Set(float64(time.Now().UnixNano()-now) / 1000000000)
}

func (e *Exporter) setMetrics(scrapes <-chan ScrapeResult) {
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

func (e *Exporter) getRecommendations() (*Recommendation, error) {

	// TODO: base instance type: from LC of ASG
	fullRecommendationUrl := fmt.Sprintf("%s/api/v1/recommender/%s", e.recommenderUrl, *e.session.Config.Region)
	res, err := http.Get(fullRecommendationUrl)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var recommendation *Recommendation
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("Couldn't get recommendations: GET '%s': response code = '%s', expecting '200 OK'", fullRecommendationUrl, res.Status))
	}
	recommendation = new(Recommendation)
	err = json.NewDecoder(res.Body).Decode(recommendation)
	if err != nil {
		return nil, err
	}
	return recommendation, nil
}

func (e *Exporter) scrapeAsg(scrapes chan<- ScrapeResult, asg *autoscaling.Group, recommendation *Recommendation) error {
	log.WithField("autoScalingGroup", *asg.AutoScalingGroupName).Debug("getting metrics from the auto scaling group")

	var pendingInstances, inServiceInstances, standbyInstances, terminatingInstances, spotInstances int
	var instanceIds []*string

	if len(asg.Instances) > 0 {
		for _, inst := range asg.Instances {
			switch *inst.LifecycleState {
			case "InService":
				inServiceInstances++
			case "Pending":
				pendingInstances++
			case "Terminating":
				terminatingInstances++
			case "Standby":
				standbyInstances++
			}
			instanceIds = append(instanceIds, inst.InstanceId)
		}
	}

	scrapes <- ScrapeResult{
		Name:             "instances_total",
		Value:            float64(len(asg.Instances)),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}
	scrapes <- ScrapeResult{
		Name:             "pending_instances_total",
		Value:            float64(pendingInstances),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}
	scrapes <- ScrapeResult{
		Name:             "inservice_instances_total",
		Value:            float64(inServiceInstances),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}
	scrapes <- ScrapeResult{
		Name:             "terminating_instances_total",
		Value:            float64(terminatingInstances),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}
	scrapes <- ScrapeResult{
		Name:             "standby_instances_total",
		Value:            float64(standbyInstances),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}

	instances, err := e.scrapeInstances(*asg.AutoScalingGroupName, instanceIds)
	if err != nil {
		return err
	}
	for _, i := range instances {
		if i.SpotBidPrice != "" {
			spotInstances++
		}
	}

	scrapes <- ScrapeResult{
		Name:             "spot_instances_total",
		Value:            float64(spotInstances),
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}

	if recommendation == nil {
		return nil
	}

	// TODO: compare it with recommendations
	scrapes <- ScrapeResult{
		Name:             "unrecommended_spot_instances_total",
		Value:            99,
		AutoScalingGroup: *asg.AutoScalingGroupName,
		Region:           *e.session.Config.Region,
	}

	return nil
}

func (e *Exporter) scrapeInstances(asgName string, instanceIds []*string) ([]Instance, error) {
	ec2Svc := ec2.New(e.session, aws.NewConfig())
	var instances = make([]Instance, 0, len(instanceIds))
	var spotRequests []*string

	err := ec2Svc.DescribeInstancesPages(&ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
	}, func(output *ec2.DescribeInstancesOutput, lastPage bool) bool {
		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				if instance.SpotInstanceRequestId != nil {
					spotRequests = append(spotRequests, instance.SpotInstanceRequestId)
				} else {
					instance := Instance{
						InstanceId:       *instance.InstanceId,
						InstanceType:     *instance.InstanceType,
						AvailabilityZone: *instance.Placement.AvailabilityZone,
					}
					instances = append(instances, instance)
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	if len(spotRequests) > 0 {
		// TODO: paginate?
		describeSpotInstanceRequestsOutput, err := ec2Svc.DescribeSpotInstanceRequests(&ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: spotRequests,
		})
		if err != nil {
			return nil, err
		}

		for _, spotRequest := range describeSpotInstanceRequestsOutput.SpotInstanceRequests {
			instance := Instance{
				InstanceId:       *spotRequest.InstanceId,
				InstanceType:     *spotRequest.LaunchSpecification.InstanceType,
				AvailabilityZone: *spotRequest.LaunchedAvailabilityZone,
				SpotBidPrice:     *spotRequest.SpotPrice,
			}
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

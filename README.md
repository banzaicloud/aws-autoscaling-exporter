## AWS Autoscaling exporter

Prometheus exporter for AWS auto scaling groups, part of the [Hollowtrees](https://github.com/banzaicloud/hollowtrees) project. Provides auto scaling group level metrics similar to CloudWatch metrics and instance level metrics for spot instances in the auto scaling group. For group level metrics the exporter is polling the AWS APIs for auto scaling groups. For instance level metrics it queries the Banzai Cloud spot instance [recommender API](https://github.com/banzaicloud/spot-recommender) to report cost and stability related metrics for spot instances.

### Quick start

Building the project is as simple as running a go build command. The result is a statically linked executable binary.
```
go build .
```

The following options can be configured when starting the exporter:

```
./aws-autoscaling-exporter --help
Usage of ./aws-autoscaling-exporter:
  -listen-address string
        The address to listen on for HTTP requests. (default ":8089")
  -log-level string
        log level (default "info")
  -metrics-path string
        path to metrics endpoint (default "/metrics")
  -recommender-url string
        URL of the spot instance recommender (default "http://localhost:9090")
  -region string
        AWS region that the exporter should query (default "eu-west-1")
```

### Metrics

```
# HELP aws_autoscaling_inservice_instances_total Total number of in service instances in the auto scaling group
# TYPE aws_autoscaling_inservice_instances_total gauge
aws_autoscaling_inservice_instances_total{asg_name="marci-test",region="eu-west-1"} 0
# HELP aws_autoscaling_instances_total Total number of instances in the auto scaling group
# TYPE aws_autoscaling_instances_total gauge
aws_autoscaling_instances_total{asg_name="marci-test",region="eu-west-1"} 1
# HELP aws_autoscaling_pending_instances_total Total number of pending instances in the auto scaling group
# TYPE aws_autoscaling_pending_instances_total gauge
aws_autoscaling_pending_instances_total{asg_name="marci-test",region="eu-west-1"} 1
# HELP aws_autoscaling_scrape_duration_seconds The scrape duration.
# TYPE aws_autoscaling_scrape_duration_seconds gauge
aws_autoscaling_scrape_duration_seconds 0.592821
# HELP aws_autoscaling_scrape_error The scrape error status.
# TYPE aws_autoscaling_scrape_error gauge
aws_autoscaling_scrape_error 0
# HELP aws_autoscaling_scrapes_total Total AWS autoscaling group scrapes.
# TYPE aws_autoscaling_scrapes_total counter
aws_autoscaling_scrapes_total 15
# HELP aws_autoscaling_spot_instances_total Total number of spot instances in the auto scaling group
# TYPE aws_autoscaling_spot_instances_total gauge
aws_autoscaling_spot_instances_total{asg_name="marci-test",region="eu-west-1"} 1
# HELP aws_autoscaling_standby_instances_total Total number of standby instances in the auto scaling group
# TYPE aws_autoscaling_standby_instances_total gauge
aws_autoscaling_standby_instances_total{asg_name="marci-test",region="eu-west-1"} 0
# HELP aws_autoscaling_terminating_instances_total Total number of terminating instances in the auto scaling group
# TYPE aws_autoscaling_terminating_instances_total gauge
aws_autoscaling_terminating_instances_total{asg_name="marci-test",region="eu-west-1"} 0
# HELP aws_instance_cost_score Current cost score of spot instance reported by the spot recommender
# TYPE aws_instance_cost_score gauge
aws_instance_cost_score{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0.787585
# HELP aws_instance_current_price Current price of spot instance reported by the spot recommender.
# TYPE aws_instance_current_price gauge
aws_instance_current_price{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0.0824
# HELP aws_instance_on_demand_price Current on demand price of spot instance reported by the spot recommender
# TYPE aws_instance_on_demand_price gauge
aws_instance_on_demand_price{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0.214
# HELP aws_instance_optimal_bid_price Optimal spot bid price of instance reported by the spot recommender
# TYPE aws_instance_optimal_bid_price gauge
aws_instance_optimal_bid_price{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0.214
# HELP aws_instance_spot_bid_price Spot bid price used to request the spot instance
# TYPE aws_instance_spot_bid_price gauge
aws_instance_spot_bid_price{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0.214
# HELP aws_instance_stability_score Current stability score of spot instance reported by the spot recommender
# TYPE aws_instance_stability_score gauge
aws_instance_stability_score{asg_name="marci-test",availability_zone="eu-west-1a",instance_id="i-061ae0a2960e194be",instance_type="m5.xlarge",region="eu-west-1"} 0
```

### Default Hollowtrees node exporters associated to alerts:

* AWS spot instance termination [collector](https://github.com/banzaicloud/spot-termination-collector)
* AWS autoscaling group [exporter](https://github.com/banzaicloud/aws-autoscaling-exporter)

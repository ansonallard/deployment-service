package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	BuildAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "deployment_service_build_attempts_total",
		Help: "Count of component build attempts by service, build type, and outcome",
	}, []string{"service", "build_type", "outcome"}) // outcome: "success" | "failure"

	BuildDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "deployment_service_build_duration_seconds",
		Help:    "Duration of component builds",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s..~2048s, tune to your actual build times
	}, []string{"service", "build_type"})

	DeployAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "deployment_service_deploy_attempts_total",
		Help: "Count of full ProcessService runs by outcome",
	}, []string{"service", "outcome"})
)

func init() {
	prometheus.MustRegister(BuildAttempts, BuildDuration, DeployAttempts)
}

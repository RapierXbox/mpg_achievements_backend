package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tokenRefreshes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "auth_token_refreshes",
			Help: "Number of access token refreshes.",
		},
	)
)

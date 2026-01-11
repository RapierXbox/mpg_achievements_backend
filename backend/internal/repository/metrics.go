package repository

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// num of qr codes
	qrCodes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "qr_qrcodes",
			Help: "Number of qr codes.",
		},
	)

	// num of qr actions
	qrActions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "qr_qractions",
			Help: "Number of qr actions.",
		},
	)

	qrScans = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "qr_qrscans",
			Help: "Number of relevant qr scans.",
		},
	)

	accounts = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "auth_accounts",
			Help: "Number of accounts.",
		},
	)

	sessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "auth_sessions",
			Help: "Number of active long lasting sessions.",
		},
	)
)

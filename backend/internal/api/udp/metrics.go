package udp

import (
	"backend/pkg/protocol"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// received packet counters
	udpPacketsReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_packets_received_total",
			Help: "Total number of UDP packets received",
		},
		[]string{"type"},
	)

	// sent packet counters
	udpPacketsSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_packets_sent_total",
			Help: "Total number of UDP packets sent",
		},
		[]string{"type"},
	)

	// received packet size metrics
	udpPacketReceivedSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "udp_packet_received_size_bytes",
			Help:    "Size of received UDP packets in bytes",
			Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096},
		},
		[]string{"type"},
	)

	udpPacketReceivedSizeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "udp_packet_received_size_bytes_summary",
			Help:       "Summary of received UDP packet sizes with percentiles",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		},
		[]string{"type"},
	)

	// sent packet size metrics
	udpPacketSentSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "udp_packet_sent_size_bytes",
			Help:    "Size of sent UDP packets in bytes",
			Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096},
		},
		[]string{"type"},
	)

	udpPacketSentSizeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "udp_packet_sent_size_bytes_summary",
			Help:       "Summary of sent UDP packet sizes with percentiles",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		},
		[]string{"type"},
	)

	// error metrics
	udpPacketErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_packet_errors_total",
			Help: "Total number of UDP packet processing errors",
		},
		[]string{"type", "error"},
	)

	// send error metrics
	udpPacketSendErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_packet_send_errors_total",
			Help: "Total number of UDP packet send errors",
		},
		[]string{"type", "error"},
	)

	// active UDP sessions
	udpActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "udp_active_sessions",
			Help: "Number of active UDP sessions",
		},
	)

	// processing duration
	udpPacketProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "udp_packet_processing_duration_seconds",
			Help:    "Time taken to process UDP packets",
			Buckets: prometheus.ExponentialBuckets(0.0001, 1.8, 20),
		},
		[]string{"type"},
	)

	// bytes transferred
	udpBytesReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_bytes_received_total",
			Help: "Total bytes received via UDP",
		},
		[]string{"type"},
	)

	udpBytesSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_bytes_sent_total",
			Help: "Total bytes sent via UDP",
		},
		[]string{"type"},
	)

	// rate limiting metrics
	udpPacketsRateLimited = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "udp_packets_rate_limited_total",
			Help: "Total number of UDP packets dropped due to rate limiting",
		},
		[]string{"type"},
	)

	// message type labels
	udpMessageTypeLabels = map[uint8]string{
		protocol.MsgTypeHello:      "hello",
		protocol.MsgTypeEntityMove: "entity_move",
		protocol.MsgTypePing:       "ping",
		protocol.MsgTypePong:       "pong",
	}
)

func getUDPMessageTypeLabel(msgType uint8) string {
	if label, ok := udpMessageTypeLabels[msgType]; ok {
		return label
	}
	return "unknown"
}

// helper function to track sent UDP packets
func trackSentUDPPacket(msgType uint8, payloadSize int) {
	typeLabel := getUDPMessageTypeLabel(msgType)
	udpPacketsSentTotal.WithLabelValues(typeLabel).Inc()

	udpPacketSentSizeBytes.WithLabelValues(typeLabel).Observe(float64(payloadSize))
	udpPacketSentSizeSummary.WithLabelValues(typeLabel).Observe(float64(payloadSize))
	udpBytesSentTotal.WithLabelValues(typeLabel).Add(float64(payloadSize))
}

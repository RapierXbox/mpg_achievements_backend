package tcp

import (
	"backend/pkg/protocol"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// received packet counters
	tcpPacketsReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_packets_received_total",
			Help: "Total number of TCP packets received",
		},
		[]string{"type"},
	)

	// sent packet counters
	tcpPacketsSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_packets_sent_total",
			Help: "Total number of TCP packets sent",
		},
		[]string{"type"},
	)

	// received packet size metrics
	tcpPacketReceivedSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tcp_packet_received_size_bytes",
			Help:    "Size of received TCP packets in bytes",
			Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536},
		},
		[]string{"type"},
	)

	tcpPacketReceivedSizeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "tcp_packet_received_size_bytes_summary",
			Help:       "Summary of received TCP packet sizes with percentiles",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		},
		[]string{"type"},
	)

	// sent packet size metrics
	tcpPacketSentSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tcp_packet_sent_size_bytes",
			Help:    "Size of sent TCP packets in bytes",
			Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536},
		},
		[]string{"type"},
	)

	tcpPacketSentSizeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "tcp_packet_sent_size_bytes_summary",
			Help:       "Summary of sent TCP packet sizes with percentiles",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.01, 0.99: 0.001},
		},
		[]string{"type"},
	)

	// connection metrics
	tcpActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "tcp_active_connections",
			Help: "Number of active TCP connections",
		},
	)

	tcpConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tcp_connections_total",
			Help: "Total number of TCP connections",
		},
	)

	// error metrics
	tcpPacketErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_packet_errors_total",
			Help: "Total number of packet processing errors",
		},
		[]string{"type", "error"},
	)

	// send error metrics
	tcpPacketSendErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_packet_send_errors_total",
			Help: "Total number of packet send errors",
		},
		[]string{"type", "error"},
	)

	// message type breakdown
	messageTypeLabels = map[uint8]string{
		protocol.MsgTypeHello:            "hello",
		protocol.MsgTypeHelloAck:         "hello_ack",
		protocol.MsgTypeNewEntity:        "new_entity",
		protocol.MsgTypeCustomData:       "custom_data",
		protocol.MsgTypeRemoveEntity:     "remove_entity",
		protocol.MsgTypeRequestChunkData: "request_chunk_data",
		protocol.MsgTypeChunkData:        "chunk_data",
		protocol.MsgTypeChatMessage:      "chat_message",
	}

	// processing duration
	tcpPacketProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tcp_packet_processing_duration_seconds",
			Help:    "Time taken to process packets",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	// bytes transferred
	tcpBytesReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_bytes_received_total",
			Help: "Total bytes received",
		},
		[]string{"type"},
	)

	tcpBytesSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_bytes_sent_total",
			Help: "Total bytes sent",
		},
		[]string{"type"},
	)
)

func getMessageTypeLabel(msgType uint8) string {
	if label, ok := messageTypeLabels[msgType]; ok {
		return label
	}
	return "unknown"
}

// helper function to track sent packets
func trackSentPacket(msgType uint8, payloadSize int) {
	typeLabel := getMessageTypeLabel(msgType)
	tcpPacketsSentTotal.WithLabelValues(typeLabel).Inc()

	tcpPacketSentSizeBytes.WithLabelValues(typeLabel).Observe(float64(payloadSize))
	tcpPacketSentSizeSummary.WithLabelValues(typeLabel).Observe(float64(payloadSize))
	tcpBytesSentTotal.WithLabelValues(typeLabel).Add(float64(payloadSize))
}

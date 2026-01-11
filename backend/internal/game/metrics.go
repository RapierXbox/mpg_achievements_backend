package game

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gameRooms = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_rooms",
			Help: "Number of rooms",
		},
	)

	// number of active game sessions
	gameSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_sessions",
			Help: "Number of active game sessions",
		},
	)

	// number of all active entities
	gameEntities = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_entities",
			Help: "Number of all active entities across all rooms",
		},
	)

	// number of all chached chunks
	gameCachedChunks = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "game_cached_chunks",
			Help: "Number of cached Chunks across all rooms",
		},
	)
)

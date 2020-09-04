package server

import (
	"github.com/rcrowley/go-metrics"
)

var (
	syncerEps = metrics.NewMeter()
	sinkEps   = metrics.NewMeter()

	proposeChannelSize = metrics.NewGauge()
)

func init() {
	metrics.Register("syncer_eps", syncerEps)
	metrics.Register("sink_eps", sinkEps)
	metrics.Register("propose_channel_size", proposeChannelSize)
}

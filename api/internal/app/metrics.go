package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "api_http_request_duration_seconds",
		Help:    "Duration of HTTP requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_http_requests_total",
		Help: "Total HTTP requests handled.",
	}, []string{"method", "path", "status"})

	MatchmakingJoins = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "api_matchmaking_joins_total",
		Help: "Total successful matchmaking join requests.",
	})

	MatchmakingInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "api_matchmaking_in_flight",
		Help: "Number of matchmaking join requests currently being processed.",
	})

	LeaderboardReads = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "api_leaderboard_reads_total",
		Help: "Total leaderboard read requests.",
	})

	LeaderboardWrites = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "api_leaderboard_writes_total",
		Help: "Total leaderboard report (write) requests.",
	})

	RedisErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "api_redis_errors_total",
		Help: "Total Redis operation errors.",
	})
)

func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		HTTPRequestDuration,
		HTTPRequestsTotal,
		MatchmakingJoins,
		MatchmakingInFlight,
		LeaderboardReads,
		LeaderboardWrites,
		RedisErrors,
	)
}

type statusCapture struct {
	http.ResponseWriter
	code int
}

func (w *statusCapture) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func WithMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusCapture{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)

		status := strconv.Itoa(sw.code)
		path := normalizePath(r.URL.Path)
		HTTPRequestDuration.WithLabelValues(r.Method, path, status).Observe(time.Since(start).Seconds())
		HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
	})
}

func normalizePath(p string) string {
	switch {
	case p == "/healthz":
		return "/healthz"
	case p == "/api/matchmaking/join":
		return "/api/matchmaking/join"
	case p == "/api/leaderboard":
		return "/api/leaderboard"
	case p == "/api/leaderboard/report":
		return "/api/leaderboard/report"
	case p == "/metrics":
		return "/metrics"
	default:
		return "/other"
	}
}

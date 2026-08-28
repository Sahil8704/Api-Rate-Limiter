// project ki main file hai
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var ctx = context.Background()

// 1. Define Prometheus metrics to track system traffic telemetry
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ratelimiter_requests_total",
			Help: "Total number of intercepted API requests categorized by client tier and status.",
		},
		[]string{"tier", "status"}, // Labels we want to chart dynamically
	)
)

func init() {
	// Register metrics with the Prometheus collector registry
	prometheus.MustRegister(httpRequestsTotal)
}

const luaScript = `
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local bucket = redis.call('HMGET', key, 'tokens', 'last_updated')
local tokens = tonumber(bucket[1])
local last_updated = tonumber(bucket[2])

if not tokens then
    tokens = max_tokens
    last_updated = now
else
    local elapsed = now - last_updated
    if elapsed > 0 then
        tokens = math.min(max_tokens, tokens + (elapsed * refill_rate))
        last_updated = now
    end
end

if tokens >= 1 then
    tokens = tokens - 1
    redis.call('HSET', key, 'tokens', tokens, 'last_updated', last_updated)
    redis.call('EXPIRE', key, 3600)
    return 1
else
    return 0
end
`

// server bnaya jo middleware ka kam krta hai
func rateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientTier := r.Header.Get("X-Client-Tier")
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)

		maxTokens := "3"
		refillRate := "1"
		tierName := "Anonymous"

		if clientTier == "Premium" {
			maxTokens = "10"
			refillRate = "5"
			tierName = "Premium"
		} else if clientTier == "Free" {
			maxTokens = "3"
			refillRate = "1"
			tierName = "Free"
		}

		redisKey := fmt.Sprintf("ratelimit:%s:%s", tierName, ip)
		now := fmt.Sprintf("%d", time.Now().Unix())

		result, err := rdb.Eval(ctx, luaScript, []string{redisKey}, maxTokens, refillRate, now).Result()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-RateLimit-Limit", maxTokens)
		w.Header().Set("X-Client-Assigned-Tier", tierName)

		if result.(int64) == 0 {
			// Telemetry Tracker: Log a blocked h
			httpRequestsTotal.WithLabelValues(tierName, "blocked").Inc()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(fmt.Sprintf(`{"error": "Too many requests", "tier": "%s"}`, tierName)))
			return
		}

		// Telemetry Tracker: Log an allowed event
		httpRequestsTotal.WithLabelValues(tierName, "allowed").Inc()
		next.ServeHTTP(w, r)
	})
}

func main() {
	// 1. Connect to the container engine by name on standard port 6379
	rdb = redis.NewClient(&redis.Options{
		Addr: "redis-db:6379",
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("Successfully connected to Distributed Redis Engine inside network layer.")

	mux := http.NewServeMux()

	// 2. Reverse Proxy Action: Forward allowed traffic to the separate core service container
	// Reverse Proxy Action: Forward allowed traffic cleanly to Nginx container
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		targetURL := "http://mock-backend:80"

		req, err := http.NewRequest(r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, "Internal Proxy Error", http.StatusInternalServerError)
			return
		}

		// Copy over client request headers
		req.Header = r.Header

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Target Service Unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers back to client
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		// Read the actual body from Nginx and stream it out cleanly
		_, _ = io.Copy(w, resp.Body)
	})

	mux.Handle("/metrics", promhttp.Handler())
	protectedServer := rateLimiterMiddleware(mux)

	fmt.Println("Distributed Traffic Cop Proxy Gateway active on port :8080...")
	if err := http.ListenAndServe(":8080", protectedServer); err != nil {
		panic(err)
	}
}

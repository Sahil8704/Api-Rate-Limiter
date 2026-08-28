# Distributed API Gateway Rate Limiter & Reverse Proxy

A high-throughput, distributed API Gateway and reverse proxy built in **Go (Golang)** designed to protect upstream backend microservices from resource exhaustion, brute-force scraping, and DDoS attacks. 

The gateway dynamically isolates clients across service tiers (`Free`, `Premium`, `Anonymous`) and executes an in-memory **Token Bucket algorithm** via atomic **Redis Lua scripts** to eliminate race conditions under concurrent load.

---


## Key Features
⚬	Atomic Concurrency Control: Executes Token Bucket refills and token subtractions as a single unbroken atomic operation inside Redis using             custom Lua scripts, preventing data race conditions during high-volume concurrency bursts.
⚬	Dynamic Multi-Tier Rate Limiting: Enforces distinct throughput constraints dynamically based on the incoming X-Client-Tier HTTP header:
⚬	Premium Tier: Capacity = 10 tokens, Refill Rate = 5 tokens/second
⚬	Free Tier: Capacity = 3 tokens, Refill Rate = 1 token/second
⚬	Anonymous (Default): Capacity = 3 tokens, Refill Rate = 1 token/second
⚬	Reverse Proxy Engine: Forwards authorized requests to an internal upstream backend service (mock-backend on Nginx), copying HTTP headers and          cleanly streaming payload bodies using Go's io.Copy.
⚬	Live System Telemetry: Implements Prometheus instrumentation (/metrics) to monitor allowed vs. blocked traffic counters tagged by client tier.
⚬	Isolated Virtual Network: Fully containerized using Docker Compose, orchestrating the Go proxy gateway, the Redis state engine, and an                upstream Nginx application over a private internal network.
        Tech Stack
⚬	Core Service: Go (Golang 1.26+)
⚬	In-Memory State Store: Redis 7 (Alpine)
⚬	Scripting Engine: Lua (Embedded Redis scripts)
⚬	Upstream Backend: Nginx (Alpine)
⚬	Telemetry & Monitoring: Prometheus Go Client (promhttp)
⚬	Infrastructure: Docker, Docker Compose

## Architecture Overview

```text
[ Client Request ] (Port 8080)
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. API GATEWAY PROXY (Go Middleware)                        │
│    - Extracts Client IP & parses 'X-Client-Tier' Header     │
│    - Assigns burst capacity & refill rates dynamically      │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. ATOMIC STATE ENGINE (Redis + Lua Script)                 │
│    - Atomic token calculation: min(max, tokens + elapsed)   │
│    - Thread-safe state update & TTL enforcement             │
└──────────────┬───────────────────────────────┬──────────────┘
               │                               │
        [ Token Available ]              [ No Tokens ]
               │                               │
               ▼                               ▼
┌───────────────────────────────┐ ┌───────────────────────────┐
│ 3. REVERSE PROXY FORWARDING   │ │ 4. RATE LIMIT ENFORCEMENT │
│  - Forwards to Nginx Backend  │ │  - Returns HTTP 429 Error │
│  - Preserves request headers  │ │  - Logs blocked metric    │
│  - Streams response via io    │ └───────────────────────────┘
└───────────────────────────────┘

// /cmd/loadgen/main.go

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	target      = flag.String("target", "http://localhost:8080", "server base URL")
	ops         = flag.Int("ops", 1000, "number of ops")
	concurrency = flag.Int("concurrency", 10, "concurrency")
	writeRatio  = flag.Float64("writeRatio", 0.5, "fraction of ops that are writes")
)

// Keep payload size constant for all experiments.
// 512 bytes is a reasonable, non-trivial size that fits comfortably in a URL.
const payloadSize = 512

func randUserID(n int) int64 {
    if n <= 0 {
        return 1
    }
    return int64(rand.Intn(n) + 1)
}

func main() {
	flag.Parse()

	transport := &http.Transport{
		MaxIdleConns:        *concurrency * 2,
		MaxIdleConnsPerHost: *concurrency,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	var wg sync.WaitGroup
	tasks := make(chan int, *ops)
	for i := 0; i < *ops; i++ {
		tasks <- i
	}
	close(tasks)

	// Pre-build constant payload (same every write).
	rawPayload := strings.Repeat("x", payloadSize)
	escapedPayload := url.QueryEscape(rawPayload)

	var writes, reads, errs uint64
	var totalLatency int64

	start := time.Now()
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range tasks {
				t0 := time.Now()
				if rand.Float64() < *writeRatio {
					id := randUserID(*ops)
					urlStr := fmt.Sprintf("%s/insert?id=%d&username=u%d&payload=%s", *target, id, id, escapedPayload)
					resp, err := client.Get(urlStr)
					lat := time.Since(t0).Nanoseconds()
					atomic.AddInt64(&totalLatency, lat)
					if err != nil {
						atomic.AddUint64(&errs, 1)
						continue
					}
					io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != 200 {
						atomic.AddUint64(&errs, 1)
						continue
					}
					atomic.AddUint64(&writes, 1)
				} else {
					id := randUserID(*ops)
					urlStr := fmt.Sprintf("%s/get?id=%d", *target, id)
					resp, err := client.Get(urlStr)
					lat := time.Since(t0).Nanoseconds()
					atomic.AddInt64(&totalLatency, lat)
					if err != nil {
						atomic.AddUint64(&errs, 1)
						continue
					}
					io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode == 200 || resp.StatusCode == 404 {
						atomic.AddUint64(&reads, 1)
					} else {
						atomic.AddUint64(&errs, 1)
					}
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	totalOps := atomic.LoadUint64(&writes) + atomic.LoadUint64(&reads) + atomic.LoadUint64(&errs)
	avgLatencyMs := float64(0)
	if totalOps > 0 {
		avgLatencyMs = float64(atomic.LoadInt64(&totalLatency)) / float64(totalOps) / 1e6
	}
	log.Printf("ops=%d elapsed=%v throughput=%.2f ops/s writes=%d reads=%d errs=%d avgLatency=%.2fms\n",
		totalOps, elapsed, float64(totalOps)/elapsed.Seconds(), writes, reads, errs, avgLatencyMs)
}

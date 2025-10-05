// /cmd/demo-server/main.go

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"os"
	"database/sql"

	"github.com/Chinzzii/sharding/internal/sharding"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    mode      = flag.String("mode", "single", "single or sharded")
    shardsNum = flag.Int("shards", 3, "number of shards (if sharded)")
    port      = flag.Int("port", 8080, "http port")
)

func main() {
	flag.Parse()

	// register metrics
	sharding.RegisterMetrics()

	// decide shard count (single -> 1)
	num := 1
	if *mode == "sharded" {
		num = *shardsNum
	}

	dbDir := os.Getenv("DEMO_DB_DIR")
	sm, err := sharding.NewShardManager(num, dbDir)
	if err != nil {
		log.Fatalf("failed to create shard manager: %v", err)
	}
	defer sm.Close()

	http.HandleFunc("/insert", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		username := r.URL.Query().Get("username")
		payload := r.URL.Query().Get("payload")

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		start := time.Now()
		if err := sm.InsertUser(id, username, payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d := time.Since(start).Seconds()
		
		sharding.WriteLatency.Observe(d)
		sharding.WriteCount.Inc()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		
		start := time.Now()
		uname, payload, err := sm.GetUser(id)
		if err == sql.ErrNoRows {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d := time.Since(start).Seconds()
		
		sharding.ReadLatency.Observe(d)
		sharding.ReadCount.Inc()
		w.Write([]byte(fmt.Sprintf("%s|%s", uname, payload)))
	})

	// metrics endpoint
	http.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("starting demo-server mode=%s shards=%d port=%s", *mode, num, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aejkatappaja/goflux/internal/analytics"
	"github.com/aejkatappaja/goflux/internal/events"
	"github.com/aejkatappaja/goflux/internal/utils"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port, tracked := parseEnv()

	bus := events.NewEventBus()
	store := analytics.NewMemoryStore()
	svc := analytics.NewService(bus, store, tracked)
	if err := svc.Start(ctx); err != nil {
		log.Fatalf("analytics start: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("GET /analytics/types", analyticsTotalsHandler(svc))
	mux.Handle("GET /analytics/types/{type}", analyticsTypeHandler(svc))
	mux.Handle("POST /events", publishEventHandler(bus))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      cors(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done() // wait for signal

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func analyticsTotalsHandler(svc *analytics.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		totals := svc.Totals() // snapshot (copie)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(totals)
	})
}

func analyticsTypeHandler(svc *analytics.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := r.PathValue("type") // variable du pattern {type}
		count := svc.TotalFor(t) // normalisation interne côté Service
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  utils.NormalizeEventType(t),
			"count": count,
		})
	})
}

type publishPayload struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	UserID  string                 `json:"userId"`
}

func publishEventHandler(bus *events.EventBus) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p publishPayload

		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		var evt events.Event
		if p.UserID == "" {
			evt = events.NewSystemEvent(p.Type, p.Payload)
		} else {
			evt = events.NewUserEvent(p.Type, p.UserID, p.Payload)
		}
		if err := bus.Publish(r.Context(), evt); err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout) // context timeout/cancel
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func parseEnv() (port string, tracked []string) {
	port = os.Getenv("PORT")
	if strings.TrimSpace(port) == "" {
		port = "8080"
	}
	raw := os.Getenv("TRACKED_TYPES")
	if strings.TrimSpace(raw) == "" {
		tracked = []string{"view", "click"}
		return
	}
	parts := strings.Split(raw, ",")
	tracked = make([]string, 0, len(parts))
	for _, p := range parts {
		if k := utils.NormalizeEventType(p); k != "" {
			tracked = append(tracked, k)
		}
	}
	return
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "600") // 10 min cache preflight

		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

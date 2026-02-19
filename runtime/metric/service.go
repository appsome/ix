package metric

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/appsome/ix/runtime/util"
)

// Service exposes a Prometheus /metrics endpoint on its own HTTP server so
// scraping is isolated from the application's request port.
type Service interface {
	Handler() *chi.Mux
	Start(context.Context, *sync.WaitGroup)
}

type service struct {
	server *http.Server
}

// New constructs a metrics service. The listen port is read from METRIC_PORT
// at Start time.
func New() Service {
	return &service{}
}

func (s *service) Handler() *chi.Mux {
	router := chi.NewRouter()
	router.Mount("/metrics", promhttp.Handler())
	return router
}

// Start launches the metrics server and registers a goroutine that shuts it
// down when ctx is cancelled, decrementing wg when done.
func (s *service) Start(ctx context.Context, wg *sync.WaitGroup) {
	if s.server == nil {
		s.server = &http.Server{
			Addr:    fmt.Sprintf(":%s", os.Getenv("METRIC_PORT")),
			Handler: s.Handler(),
		}
	}

	log.Printf("Starting metrics service")
	wg.Add(1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			RecordError("METRIC001", "metrics service failed", err)
		}
	}()

	go func() {
		<-ctx.Done()
		s.shutdown()
		wg.Done()
	}()
}

func (s *service) shutdown() {
	timeout := util.GetEnvInt32("SHUTDOWN_TIMEOUT", 20)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		RecordError("METRIC002", "metrics service shutdown failed", err)
	}
}

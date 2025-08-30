package http

import (
	"context"
	"errors"
	"log/slog"
	nhttp "net/http"
	"time"

	"go-demo/internal/config"
)

type Server struct {
	http *nhttp.Server
	log  *slog.Logger
}

func NewServer(cfg config.Config, h nhttp.Handler, log *slog.Logger) *Server {
	s := &Server{
		http: &nhttp.Server{
			Addr:              ":" + cfg.Port,
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		log: log,
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server starting", "addr", s.http.Addr)
		errCh <- s.http.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		s.log.Info("http server shutting down")
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.http.Shutdown(c); err != nil {
			s.log.Error("http server shutdown error", "err", err)
			return err
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, nhttp.ErrServerClosed) {
			return nil
		}
		s.log.Error("http server error", "err", err)
		return err
	}
}

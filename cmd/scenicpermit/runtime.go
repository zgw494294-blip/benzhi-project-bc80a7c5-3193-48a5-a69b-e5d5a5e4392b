package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"scenicpermit/internal/application"
	"scenicpermit/internal/httpapi"
	"scenicpermit/internal/persistence"
)

type runtime struct {
	store    *persistence.Store
	server   *http.Server
	listener net.Listener
	logger   *slog.Logger
}

func newRuntime(address, dataPath string, logger *slog.Logger) (*runtime, error) {
	store, err := persistence.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("打开本地数据存储失败: %w", err)
	}
	service := application.NewService(store)
	api := httpapi.New(service, logger)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("监听 %s 失败: %w", address, err)
	}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return &runtime{store: store, server: server, listener: listener, logger: logger}, nil
}

func (r *runtime) serve(errorChannel chan<- error) {
	r.logger.Info("舞台布景阻燃准用工作台已启动", "address", r.listener.Addr().String())
	err := r.server.Serve(r.listener)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	errorChannel <- err
}

func (r *runtime) shutdown(ctx context.Context) error {
	serverErr := r.server.Shutdown(ctx)
	storeErr := r.store.Close()
	return errors.Join(serverErr, storeErr)
}

func runService(cfg config, logger *slog.Logger) error {
	rt, err := newRuntime(cfg.Address, cfg.DataPath, logger)
	if err != nil {
		return err
	}
	errorsFromServer := make(chan error, 1)
	go rt.serve(errorsFromServer)
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errorsFromServer:
		if err != nil {
			return err
		}
		return rt.store.Close()
	case <-signalContext.Done():
		logger.Info("收到终止信号，正在优雅关闭")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return rt.shutdown(ctx)
	}
}

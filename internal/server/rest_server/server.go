package rest_server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/okieraised/monitoring-agent/internal/config"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/middlewares"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func getHTTPPort() int {
	port := viper.GetInt(config.AgentHTTPPort)
	if port <= 0 {
		return constants.AgentDefaultHTTPPort
	}
	return port
}

func getHTTPRequestTimeout() time.Duration {
	timeout := constants.DefaultHTTPRequestTimeout
	if viper.GetInt(config.AgentHTTPRequestTimeout) > 0 {
		timeout = viper.GetInt(config.AgentHTTPRequestTimeout)
	}

	return time.Duration(timeout) * time.Second
}

func NewHTTPServer(ctx context.Context, registerRoutes func(engine *gin.Engine)) error {
	log.Default().Info("Initializing HTTP server")

	gin.SetMode(viper.GetString(config.AgentHTTPMode))
	router := gin.New()
	router.Use(gin.Recovery())

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodGet, http.MethodDelete},
		AllowHeaders: []string{constants.HeaderAccessControlAllowHeaders, constants.HeaderOrigin, constants.HeaderAccept,
			constants.HeaderXRequestedWith, constants.HeaderContentType, constants.HeaderAuthorization, constants.HeaderXAPIKey},
		ExposeHeaders:    []string{constants.HeaderContentLength},
		AllowCredentials: true,
	}))

	router.NoRoute(middlewares.NoRouteMW())
	router.Use(middlewares.RequestIDMW())

	if registerRoutes != nil {
		registerRoutes(router)
	}

	router.Use(
		middlewares.RequestTimeoutMW(getHTTPRequestTimeout()),
		gzip.Gzip(gzip.DefaultCompression),
		middlewares.RecoveryMW(),
		middlewares.RequestLoggingMW(log.Default().Logger),
		middlewares.ResponseHashMW(),
	)

	serverAddr := fmt.Sprintf("0.0.0.0:%d", getHTTPPort())
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if viper.GetString(config.AgentTLSCertFile) != "" && viper.GetString(config.AgentTLSKeyFile) != "" {
			err = srv.ListenAndServeTLS(viper.GetString(config.AgentTLSCertFile), viper.GetString(config.AgentTLSKeyFile))
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Default().Info("Shutting down HTTP server")
		stopped := make(chan struct{})
		go func() {
			err := srv.Shutdown(ctx)
			if err != nil {
				wErr := errors.Wrap(err, "failed to shutdown http server")
				log.Default().Error(wErr.Error())
			}
			close(stopped)
		}()

		t := time.NewTimer(3 * time.Second)
		defer t.Stop()
		select {
		case <-stopped:
			return nil
		case <-t.C:
			log.Default().Info("Graceful stop timed out, forcing shutdown")
			_ = srv.Close()
			return nil
		}
	case err := <-errCh:
		wErr := errors.Wrap(err, "failed to start HTTP server")
		return wErr
	}
}

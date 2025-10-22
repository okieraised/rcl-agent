package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/okieraised/monitoring-agent/internal/agent/ros_topics_retriever"
	"github.com/okieraised/monitoring-agent/internal/config"
	"github.com/okieraised/monitoring-agent/internal/constants"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/local_cache"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/mqtt_client"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/ros_node"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/s3_client"
	"github.com/okieraised/monitoring-agent/internal/infrastructure/tracer_client"
	"github.com/okieraised/monitoring-agent/internal/server/grpc_server"
	grpc_routes "github.com/okieraised/monitoring-agent/internal/server/grpc_server/routes"
	"github.com/okieraised/monitoring-agent/internal/server/grpc_server/services"
	"github.com/okieraised/monitoring-agent/internal/server/monitoring"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/routers"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/restful"
	"github.com/okieraised/monitoring-agent/internal/server/rest_server/services/v1/ws"
	"github.com/okieraised/monitoring-agent/internal/signaling"
	"github.com/okieraised/monitoring-agent/internal/webrtc"
	"github.com/okieraised/rclgo/jazzy"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var once sync.Once

func mirrorEnvCase() {
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		k, v := kv[:i], kv[i+1:]
		_ = os.Setenv(strings.ToUpper(k), v)
		_ = os.Setenv(strings.ToLower(k), v)
	}
}

func loadDotenvIfExists(filename string, overload bool) (bool, error) {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if overload {
		return true, godotenv.Overload(filename)
	}
	return true, godotenv.Load(filename)
}

func readConfigIfExists(path string, merge bool) (bool, error) {
	viper.SetConfigFile(path)
	var err error
	if merge {
		err = viper.MergeInConfig()
	} else {
		err = viper.ReadInConfig()
	}
	if err == nil {
		return true, nil
	}
	var nf viper.ConfigFileNotFoundError
	if errors.As(err, &nf) || os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func detectProfile() string {
	from := func(k string) (string, bool) {
		if v, ok := os.LookupEnv(k); ok {
			return strings.ToLower(v), true
		}
		if v, ok := os.LookupEnv(strings.ToUpper(k)); ok {
			return strings.ToLower(v), true
		}
		if v, ok := os.LookupEnv(strings.ToLower(k)); ok {
			return strings.ToLower(v), true
		}
		return "", false
	}
	if v, ok := from("APP_ENV"); ok {
		return v
	}
	return "dev"
}

func Load() error {
	envFound, err := loadDotenvIfExists(".env", false)
	if err != nil {
		return err
	}
	if envFound {
		mirrorEnvCase()
	}
	profile := detectProfile()

	if pfFound, err := func() (bool, error) {
		found, e := loadDotenvIfExists("."+profile+".env", true)
		if found {
			mirrorEnvCase()
		}
		return found, e
	}(); err != nil {
		return err
	} else if pfFound {
	}

	cfgFound, err := readConfigIfExists("conf/config.toml", false)
	if err != nil {
		return err
	}

	if !envFound && !cfgFound {
		return fmt.Errorf("no configuration sources found: missing both .env and conf/config.toml")
	}

	if _, err := readConfigIfExists("conf/"+profile+".config.toml", true); err != nil {
		return err
	}

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	viper.AutomaticEnv()

	return nil
}

func init() {
	once.Do(func() {
		err := Load()
		if err != nil {
			panic(fmt.Sprintf("Failed to setup service configuration: %v", err))
		}

		// Init default logger
		err = log.InitDefault()
		if err != nil {
			panic(err)
		}

		// Initialize websocket hub
		websocketHub := signaling.NewWebsocketHub()
		websocketHub.Run(context.Background())

		if viper.GetBool(config.AgentEnableS3) {
			log.Default().Info("Started initializing client connection to external S3 storage")
			err = s3_client.NewS3Client(
				context.Background(),
				s3_client.WithRegion(viper.GetString(config.S3Region)),
				s3_client.WithEndpoint(viper.GetString(config.S3Endpoint), viper.GetBool(config.S3UsePathStyle)),
				s3_client.WithStaticCredentials(config.S3AccessKey, config.S3SecretKey, ""),
				s3_client.WithRetry(5, 30*time.Second),
				s3_client.WithHTTPClient(
					&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{
								InsecureSkipVerify: viper.GetBool(config.S3TLSInsecureSkipVerify),
							},
						},
					},
				),
			)
			if err != nil {
				log.Default().Fatal(fmt.Sprintf("Failed to initialize client connection to external S3 storage: %v", err))
			}
			log.Default().Info("Finished initializing client connection to external S3 storage")
		}

		// Initialize MQTT client if enabled
		if viper.GetBool(config.AgentEnableMQTT) {
			log.Default().Info("Started initializing client connection to MQTT broker")
			err = mqtt_client.NewMQTTClient(
				viper.GetString(config.MqttEndpoint),
				viper.GetString(config.MqttClientId),
				mqtt_client.WithAutoReconnect(viper.GetBool(config.MqttAutoReconnect)),
				mqtt_client.WithConnectTimeout(5*time.Second),
				mqtt_client.WithTLSInsecureSkipVerify(viper.GetBool(config.MqttTLSInsecureSkipVerify)),
			)
			if err != nil {
				log.Default().Fatal(fmt.Sprintf("Failed to initialize client connection to MQTT broker: %v", err))
			}
			log.Default().Info("Finished initializing client connection to MQTT broker")
		}

		// Initialize OTEL tracer if enabled
		if viper.GetBool(config.AgentEnableTracing) {
			log.Default().Info("Started initializing OTEL tracer")
			_, err = tracer_client.NewTracerClient()
			if err != nil {
				log.Default().Fatal(fmt.Sprintf("Failed to initialize OTEL tracer: %v", err))
			}
			log.Default().Info("Finished initializing OTEL tracer")
		}

		// Initialize local cache
		log.Default().Info("Started initializing local cache")
		err = local_cache.NewLocalCache()
		if err != nil {
			log.Default().Fatal(fmt.Sprintf("Failed to initialize local cache: %v", err))
		}
		log.Default().Info("Finished initializing local cache")
		log.Default().Info("Finished initializing connection to external services")
	})
}

func main() {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	defer signal.Stop(sigCh)

	// Init ROS
	err := jazzy.Init(nil)
	if err != nil {
		log.Default().Fatal(fmt.Sprintf("Failed to initialize ROS client: %v", err))
		return
	}
	defer func() {
		cErr := jazzy.Deinit()
		if cErr != nil && err == nil {
			err = cErr
		}
	}()

	// Init new node
	rosNode, err := ros_node.NewRosNode()
	if err != nil {
		log.Default().Fatal(fmt.Sprintf("Failed to initialize ROS client: %v", err))
		return
	}
	defer func(node *jazzy.Node) {
		cErr := node.Close()
		if cErr != nil && err == nil {
			err = cErr
		}
	}(rosNode)

	// Init other services
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(parentCtx)

	// Init ROS2 topic retriever
	g.Go(func() error {
		trErr := ros_topics_retriever.NewROS2TopicRetriever(ctx)
		if trErr != nil {
			return trErr
		}
		return ctx.Err()
	})

	// Init GRPC server
	g.Go(func() error {
		gErr := grpc_server.NewGRPCServer(ctx, func(s *grpc.Server) {
			grpc_routes.RegisterROSInterfaceServer(s, &services.ROSInterfaceService{})
		})
		if gErr != nil {
			return gErr
		}
		return ctx.Err()
	})

	// Init profiling
	g.Go(func() error {
		if viper.GetBool(config.AgentEnableMonitoring) {
			mErr := monitoring.NewMonitoringServer(ctx)
			if mErr != nil {
				return mErr
			}
		}

		return ctx.Err()
	})

	// Init HTTP server
	g.Go(func() error {
		// app state
		appState := routers.NewAppState()

		// v1 restful svc
		v1RestState := routers.NewV1RestState()
		v1RestState.SetWebRTCService(
			restful.NewWebRTCService(),
		)
		v1RestState.SetHealthcheckService(
			restful.NewHealthcheckService(),
		)
		appState.SetV1RestState(v1RestState)

		websocketState := routers.NewWebsocketState()
		websocketState.SetWebsocketService(
			ws.NewWebsocketService(
				ws.WithWebsocketHub(signaling.GetWebsocketHubInstance()),
			),
		)
		appState.SetWebsocketState(websocketState)

		rErr := rest_server.NewHTTPServer(ctx, routers.NewRootRouter(appState).InitRouters)
		if rErr != nil {
			return rErr
		}
		return ctx.Err()
	})

	// Init WebRTC viewer
	g.Go(func() error {
		cWebRTC, rtcErr := webrtc.NewWebRTCDaemon(parentCtx)
		if rtcErr != nil {
			wErr := errors.Wrap(err, "failed to initialize new WebRTC service")
			log.Default().Fatal(wErr.Error())
			return wErr
		}
		defer cWebRTC.Close()

		rtcErr = cWebRTC.Start()
		if rtcErr != nil {
			log.Default().Fatal(fmt.Sprintf("Failed to start ROS WebRTC: %v", err))
			return rtcErr
		}
		return ctx.Err()
	})

	select {
	case sig := <-sigCh:
		log.Default().Debug(fmt.Sprintf("Signal received: %v", sig))
		cancel()

		done := make(chan error, 1)
		go func() {
			done <- g.Wait()
		}()

		select {
		case err = <-done:
			log.Default().Info("All tasks exited, shutting down agent")
			return
		case sig2 := <-sigCh:
			log.Default().Debug(fmt.Sprintf("Second signal received: %v", sig2))
			return
		case <-time.After(constants.GraceWaitPeriod):
			log.Default().Info("Grace period timed out, forcing exit")
			return
		}

	case err = <-func() chan error {
		ch := make(chan error, 1)
		go func() {
			ch <- g.Wait()
		}()
		return ch
	}():
		log.Default().Info(fmt.Sprintf("Services finished early with error: %v", err))
	}
}

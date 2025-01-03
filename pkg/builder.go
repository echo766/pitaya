package pitaya

import (
	"strconv"

	"github.com/echo766/pitaya/pkg/acceptor"
	"github.com/echo766/pitaya/pkg/agent"
	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/config"
	"github.com/echo766/pitaya/pkg/conn/codec"
	"github.com/echo766/pitaya/pkg/conn/message"
	"github.com/echo766/pitaya/pkg/constants"
	"github.com/echo766/pitaya/pkg/defaultpipelines"
	"github.com/echo766/pitaya/pkg/groups"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/metrics"
	"github.com/echo766/pitaya/pkg/metrics/models"
	"github.com/echo766/pitaya/pkg/pipeline"
	"github.com/echo766/pitaya/pkg/router"
	"github.com/echo766/pitaya/pkg/serialize"
	"github.com/echo766/pitaya/pkg/serialize/json"
	"github.com/echo766/pitaya/pkg/service"
	"github.com/echo766/pitaya/pkg/session"
	"github.com/echo766/pitaya/pkg/worker"
	"github.com/google/uuid"
)

// Builder holds dependency instances for a pitaya App
type Builder struct {
	acceptors        []acceptor.Acceptor
	Config           config.BuilderConfig
	DieChan          chan bool
	PacketDecoder    codec.PacketDecoder
	PacketEncoder    codec.PacketEncoder
	MessageEncoder   *message.MessagesEncoder
	Serializer       serialize.Serializer
	Router           *router.Router
	RPCClient        cluster.RPCClient
	RPCServer        cluster.RPCServer
	MetricsReporters []metrics.Reporter
	Server           *cluster.Server
	ServerMode       ServerMode
	ServiceDiscovery cluster.ServiceDiscovery
	Groups           groups.GroupService
	SessionPool      session.SessionPool
	Worker           *worker.Worker
	HandlerHooks     *pipeline.HandlerHooks
	zoneId           string
}

// PitayaBuilder Builder interface
type PitayaBuilder interface {
	Build() Pitaya
}

// NewBuilderWithConfigs return a builder instance with default dependency instances for a pitaya App
// with configs defined by a config file (config.Config) and default paths (see documentation).
func NewBuilderWithConfigs(
	isFrontend bool,
	serverType string,
	serverMode ServerMode,
	serverMetadata map[string]string,
	conf *config.Config,
) *Builder {
	builderConfig := config.NewBuilderConfig(conf)
	customMetrics := config.NewCustomMetricsSpec(conf)
	prometheusConfig := config.NewPrometheusConfig(conf)
	statsdConfig := config.NewStatsdConfig(conf)
	etcdSDConfig := config.NewEtcdServiceDiscoveryConfig(conf)
	natsRPCServerConfig := config.NewNatsRPCServerConfig(conf)
	natsRPCClientConfig := config.NewNatsRPCClientConfig(conf)
	workerConfig := config.NewWorkerConfig(conf)
	enqueueOpts := config.NewEnqueueOpts(conf)
	groupServiceConfig := config.NewMemoryGroupConfig(conf)
	return NewBuilder(
		isFrontend,
		serverType,
		serverMode,
		serverMetadata,
		*builderConfig,
		*customMetrics,
		*prometheusConfig,
		*statsdConfig,
		*etcdSDConfig,
		*natsRPCServerConfig,
		*natsRPCClientConfig,
		*workerConfig,
		*enqueueOpts,
		*groupServiceConfig,
	)
}

// NewDefaultBuilder return a builder instance with default dependency instances for a pitaya App,
// with default configs
func NewDefaultBuilder(isFrontend bool, serverType string, serverMode ServerMode, serverMetadata map[string]string, builderConfig config.BuilderConfig) *Builder {
	customMetrics := config.NewDefaultCustomMetricsSpec()
	prometheusConfig := config.NewDefaultPrometheusConfig()
	statsdConfig := config.NewDefaultStatsdConfig()
	etcdSDConfig := config.NewDefaultEtcdServiceDiscoveryConfig()
	natsRPCServerConfig := config.NewDefaultNatsRPCServerConfig()
	natsRPCClientConfig := config.NewDefaultNatsRPCClientConfig()
	workerConfig := config.NewDefaultWorkerConfig()
	enqueueOpts := config.NewDefaultEnqueueOpts()
	groupServiceConfig := config.NewDefaultMemoryGroupConfig()
	return NewBuilder(
		isFrontend,
		serverType,
		serverMode,
		serverMetadata,
		builderConfig,
		*customMetrics,
		*prometheusConfig,
		*statsdConfig,
		*etcdSDConfig,
		*natsRPCServerConfig,
		*natsRPCClientConfig,
		*workerConfig,
		*enqueueOpts,
		*groupServiceConfig,
	)
}

// NewBuilder return a builder instance with default dependency instances for a pitaya App,
// with configs explicitly defined
func NewBuilder(isFrontend bool,
	serverType string,
	serverMode ServerMode,
	serverMetadata map[string]string,
	config config.BuilderConfig,
	customMetrics models.CustomMetricsSpec,
	prometheusConfig config.PrometheusConfig,
	statsdConfig config.StatsdConfig,
	etcdSDConfig config.EtcdServiceDiscoveryConfig,
	natsRPCServerConfig config.NatsRPCServerConfig,
	natsRPCClientConfig config.NatsRPCClientConfig,
	workerConfig config.WorkerConfig,
	enqueueOpts config.EnqueueOpts,
	groupServiceConfig config.MemoryGroupConfig,
) *Builder {
	server := cluster.NewServer(uuid.New().String(), serverType, isFrontend, serverMetadata)
	dieChan := make(chan bool)

	metricsReporters := []metrics.Reporter{}
	if config.Metrics.Prometheus.Enabled {
		metricsReporters = addDefaultPrometheus(prometheusConfig, customMetrics, metricsReporters, serverType)
	}

	if config.Metrics.Statsd.Enabled {
		metricsReporters = addDefaultStatsd(statsdConfig, metricsReporters, serverType)
	}

	handlerHooks := pipeline.NewHandlerHooks()
	if config.DefaultPipelines.StructValidation.Enabled {
		configureDefaultPipelines(handlerHooks)
	}

	sessionPool := session.NewSessionPool()

	var serviceDiscovery cluster.ServiceDiscovery
	var rpcServer cluster.RPCServer
	var rpcClient cluster.RPCClient
	if serverMode == Cluster {
		var err error
		serviceDiscovery, err = cluster.NewEtcdServiceDiscovery(etcdSDConfig, server, dieChan)
		if err != nil {
			logger.Log.Fatalf("error creating default cluster service discovery component: %s", err.Error())
		}

		rpcServer, err = cluster.NewNatsRPCServer(natsRPCServerConfig, server, metricsReporters, dieChan, sessionPool)
		if err != nil {
			logger.Log.Fatalf("error setting default cluster rpc server component: %s", err.Error())
		}

		rpcClient, err = cluster.NewNatsRPCClient(natsRPCClientConfig, server, metricsReporters, dieChan)
		if err != nil {
			logger.Log.Fatalf("error setting default cluster rpc client component: %s", err.Error())
		}
	}

	worker, err := worker.NewWorker(workerConfig, enqueueOpts)
	if err != nil {
		logger.Log.Fatalf("error creating default worker: %s", err.Error())
	}

	gsi := groups.NewMemoryGroupService(groupServiceConfig)
	if err != nil {
		logger.Log.Fatal(err)
	}

	return &Builder{
		acceptors:        []acceptor.Acceptor{},
		Config:           config,
		DieChan:          dieChan,
		PacketDecoder:    codec.NewPomeloPacketDecoder(),
		PacketEncoder:    codec.NewPomeloPacketEncoder(),
		MessageEncoder:   message.NewMessagesEncoder(config.Pitaya.Handler.Messages.Compression),
		Serializer:       json.NewSerializer(),
		Router:           router.New(),
		RPCClient:        rpcClient,
		RPCServer:        rpcServer,
		MetricsReporters: metricsReporters,
		Server:           server,
		ServerMode:       serverMode,
		Groups:           gsi,
		HandlerHooks:     handlerHooks,
		ServiceDiscovery: serviceDiscovery,
		SessionPool:      sessionPool,
		Worker:           worker,
		zoneId:           serverMetadata[constants.ZoneIDKey],
	}
}

// AddAcceptor adds a new acceptor to app
func (builder *Builder) AddAcceptor(ac acceptor.Acceptor) {
	if !builder.Server.Frontend {
		logger.Log.Error("tried to add an acceptor to a backend server, skipping")
		return
	}
	builder.acceptors = append(builder.acceptors, ac)
}

// Build returns a valid App instance
func (builder *Builder) Build() Pitaya {
	handlerPool := service.NewHandlerPool()
	var remoteService *service.RemoteService
	if builder.ServerMode == Standalone {
		if builder.ServiceDiscovery != nil || builder.RPCClient != nil || builder.RPCServer != nil {
			logger.Log.Fatal("Standalone mode can't have RPC or service discovery instances")
		}
	} else {
		if !(builder.ServiceDiscovery != nil && builder.RPCClient != nil && builder.RPCServer != nil) {
			logger.Log.Fatal("Cluster mode must have RPC and service discovery instances")
		}

		remoteService = service.NewRemoteService(
			builder.RPCClient,
			builder.RPCServer,
			builder.ServiceDiscovery,
			builder.PacketEncoder,
			builder.Serializer,
			builder.Router,
			builder.MessageEncoder,
			builder.Server,
			builder.SessionPool,
			builder.HandlerHooks,
			handlerPool,
		)

		builder.RPCServer.SetPitayaServer(remoteService)
	}

	agentFactory := agent.NewAgentFactory(builder.DieChan,
		builder.PacketDecoder,
		builder.PacketEncoder,
		builder.Serializer,
		builder.Config.Pitaya.Heartbeat.Interval,
		builder.MessageEncoder,
		builder.Config.Pitaya.Buffer.Agent.Messages,
		builder.SessionPool,
		builder.MetricsReporters,
	)

	zoneId, _ := strconv.Atoi(builder.zoneId)

	handlerService := service.NewHandlerService(
		builder.PacketDecoder,
		builder.Serializer,
		builder.Config.Pitaya.Buffer.Handler.LocalProcess,
		builder.Config.Pitaya.Buffer.Handler.RemoteProcess,
		builder.Server,
		remoteService,
		agentFactory,
		builder.MetricsReporters,
		builder.HandlerHooks,
		handlerPool,
		builder.Config.Pitaya.Concurrency.Handler.Dispatch,
		uint32(zoneId),
		builder.Router,
	)

	return NewApp(
		builder.ServerMode,
		builder.Serializer,
		builder.acceptors,
		builder.DieChan,
		builder.Router,
		builder.Server,
		builder.RPCClient,
		builder.RPCServer,
		builder.Worker,
		builder.ServiceDiscovery,
		remoteService,
		handlerService,
		builder.Groups,
		builder.SessionPool,
		builder.MetricsReporters,
		builder.Config.Pitaya,
	)
}

// NewDefaultApp returns a default pitaya app instance
func NewDefaultApp(isFrontend bool, serverType string, serverMode ServerMode, serverMetadata map[string]string, config config.BuilderConfig) Pitaya {
	builder := NewDefaultBuilder(isFrontend, serverType, serverMode, serverMetadata, config)
	return builder.Build()
}

func configureDefaultPipelines(handlerHooks *pipeline.HandlerHooks) {
	handlerHooks.BeforeHandler.PushBack(defaultpipelines.StructValidatorInstance.Validate)
}

func addDefaultPrometheus(config config.PrometheusConfig, customMetrics models.CustomMetricsSpec, reporters []metrics.Reporter, serverType string) []metrics.Reporter {
	prometheus, err := CreatePrometheusReporter(serverType, config, customMetrics)
	if err != nil {
		logger.Log.Errorf("failed to start prometheus metrics reporter, skipping %v", err)
	} else {
		reporters = append(reporters, prometheus)
	}
	return reporters
}

func addDefaultStatsd(config config.StatsdConfig, reporters []metrics.Reporter, serverType string) []metrics.Reporter {
	statsd, err := CreateStatsdReporter(serverType, config)
	if err != nil {
		logger.Log.Errorf("failed to start statsd metrics reporter, skipping %v", err)
	} else {
		reporters = append(reporters, statsd)
	}
	return reporters
}

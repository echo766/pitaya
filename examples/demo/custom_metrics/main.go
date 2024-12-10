package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/echo766/pitaya/examples/demo/custom_metrics/services"
	pitaya "github.com/echo766/pitaya/pkg"
	"github.com/echo766/pitaya/pkg/acceptor"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/config"
	"github.com/spf13/viper"
)

var app pitaya.Pitaya

func main() {
	port := flag.Int("port", 3250, "the port to listen")
	svType := "room"
	isFrontend := true

	flag.Parse()

	cfg := viper.New()
	cfg.AddConfigPath(".")
	cfg.SetConfigName("config")
	err := cfg.ReadInConfig()
	if err != nil {
		panic(err)
	}

	tcp := acceptor.NewTCPAcceptor(fmt.Sprintf(":%d", *port))

	conf := config.NewConfig(cfg)
	builder := pitaya.NewBuilderWithConfigs(isFrontend, svType, pitaya.Cluster, map[string]string{}, conf)
	builder.AddAcceptor(tcp)
	app = builder.Build()

	defer app.Shutdown()

	app.Register(services.NewRoom(app),
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
		component.WithHandler(),
	)

	app.Start()
}

package main

import (
	"flag"
	"fmt"

	"strings"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/acceptor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/examples/demo/custom_metrics/services"
	"github.com/echo766/pitaya/serialize/json"
	"github.com/spf13/viper"
)

func configureRoom(port int) {
	tcp := acceptor.NewTCPAcceptor(fmt.Sprintf(":%d", port))
	pitaya.AddAcceptor(tcp)

	pitaya.Register(&services.Room{},
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
	)
}

func main() {
	port := flag.Int("port", 3250, "the port to listen")
	svType := "room"
	isFrontend := true

	flag.Parse()

	defer pitaya.Shutdown()

	pitaya.SetSerializer(json.NewSerializer())

	config := viper.New()
	config.AddConfigPath(".")
	config.SetConfigName("config")
	err := config.ReadInConfig()
	if err != nil {
		panic(err)
	}

	pitaya.Configure(isFrontend, svType, pitaya.Cluster, map[string]string{}, config)
	configureRoom(*port)
	pitaya.Start()
}

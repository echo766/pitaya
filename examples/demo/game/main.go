package main

import (
	"log"
	"time"

	"strings"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/acceptor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/config"
	"github.com/echo766/pitaya/examples/demo/game/logic/avatar"
)

func main() {
	defer pitaya.Shutdown()

	conf := configApp()

	builder := pitaya.NewDefaultBuilder(true, "game", pitaya.Standalone, map[string]string{}, *conf)
	builder.AddAcceptor(acceptor.NewWSAcceptor(":3250"))

	app := builder.Build()

	app.Register(avatar.NewAvatarMgr(),
		component.WithName("avatarmgr"),
		component.WithNameFunc(strings.ToLower),
	)

	log.SetFlags(log.LstdFlags | log.Llongfile)
	app.Start()
}

func configApp() *config.BuilderConfig {
	conf := config.NewDefaultBuilderConfig()
	conf.Pitaya.Buffer.Handler.LocalProcess = 15
	conf.Pitaya.Heartbeat.Interval = time.Duration(15 * time.Second)
	conf.Pitaya.Buffer.Agent.Messages = 32
	conf.Pitaya.Handler.Messages.Compression = false
	return conf
}

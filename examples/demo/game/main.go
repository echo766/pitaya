package main

import (
	"log"

	"strings"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/acceptor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/examples/demo/game/logic/avatar"
	"github.com/echo766/pitaya/examples/demo/game/logic/gate"
	"github.com/echo766/pitaya/serialize/json"
	"github.com/spf13/viper"
)

func main() {
	defer pitaya.Shutdown()

	conf := configApp()

	pitaya.SetSerializer(json.NewSerializer())

	pitaya.Register(avatar.NewAvatarMgr(),
		component.WithName("avatarmgr"),
		component.WithNameFunc(strings.ToLower),
	)

	pitaya.RegisterModule(gate.NewRouteMgr(), "route")

	log.SetFlags(log.LstdFlags | log.Llongfile)

	t := acceptor.NewWSAcceptor(":3250")
	pitaya.AddAcceptor(t)

	pitaya.Configure(true, "game", pitaya.Standalone, map[string]string{}, conf)
	pitaya.Start()
}

func configApp() *viper.Viper {
	conf := viper.New()
	conf.SetEnvPrefix("game") // allows using env vars in the CHAT_PITAYA_ format
	conf.SetDefault("pitaya.buffer.handler.localprocess", 15)
	conf.Set("pitaya.heartbeat.interval", "15s")
	conf.Set("pitaya.buffer.agent.messages", 32)
	conf.Set("pitaya.handler.messages.compression", false)
	return conf
}

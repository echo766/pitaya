package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/echo766/pitaya/examples/game/comp/chat"
	pitaya "github.com/echo766/pitaya/pkg"
	"github.com/echo766/pitaya/pkg/acceptor"
	"github.com/echo766/pitaya/pkg/async"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/config"
	"github.com/echo766/pitaya/pkg/groups"
)

var app pitaya.Pitaya

func main() {
	conf := configApp()
	builder := pitaya.NewDefaultBuilder(true, "chat", pitaya.Standalone, map[string]string{}, *conf)
	builder.AddAcceptor(acceptor.NewWSAcceptor(":3250"))
	builder.Groups = groups.NewMemoryGroupService(*config.NewDefaultMemoryGroupConfig())
	app = builder.Build()

	defer app.Shutdown()

	err := app.GroupCreate(context.Background(), "room")
	if err != nil {
		panic(err)
	}

	// rewrite component and handler name
	room := chat.NewRoom(app)
	app.Register(room,
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
		component.WithHandler(),
	)

	log.SetFlags(log.LstdFlags | log.Llongfile)

	http.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))

	async.GoRaw(func() {
		http.ListenAndServe(":3251", nil)
	})

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

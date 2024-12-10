package main

import (
	"context"
	"flag"
	"fmt"

	"strings"

	"github.com/echo766/pitaya/examples/demo/cluster/services"
	pitaya "github.com/echo766/pitaya/pkg"
	"github.com/echo766/pitaya/pkg/acceptor"
	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/config"
	"github.com/echo766/pitaya/pkg/groups"
	"github.com/echo766/pitaya/pkg/protos"
	"github.com/echo766/pitaya/pkg/route"
)

var app pitaya.Pitaya

func configureBackend() {
	room := services.NewRoom(app)
	app.Register(room,
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
		component.WithHandler(),
		component.WithRemote(),
	)
}

func configureFrontend(port int) {
	app.Register(services.NewConnector(app),
		component.WithName("connector"),
		component.WithNameFunc(strings.ToLower),
		component.WithHandler(),
	)

	app.Register(services.NewConnectorRemote(app),
		component.WithName("connectorremote"),
		component.WithNameFunc(strings.ToLower),
		component.WithRemote(),
	)

	err := app.AddRoute("room", func(
		ctx context.Context,
		route *route.Route,
		payload []byte,
		rpcType protos.RPCType,
	) (*cluster.Server, error) {
		// will return the first server
		// todo
		return nil, nil
	})

	if err != nil {
		fmt.Printf("error adding route %s\n", err.Error())
	}

	err = app.SetDictionary(map[string]uint16{
		"connector.getsessiondata": 1,
		"connector.setsessiondata": 2,
		"room.room.getsessiondata": 3,
		"onMessage":                4,
		"onMembers":                5,
	})

	if err != nil {
		fmt.Printf("error setting route dictionary %s\n", err.Error())
	}
}

func main() {
	port := flag.Int("port", 3250, "the port to listen")
	svType := flag.String("type", "connector", "the server type")
	isFrontend := flag.Bool("frontend", true, "if server is frontend")

	flag.Parse()

	builder := pitaya.NewDefaultBuilder(*isFrontend, *svType, pitaya.Cluster, map[string]string{}, *config.NewDefaultBuilderConfig())
	if *isFrontend {
		tcp := acceptor.NewTCPAcceptor(fmt.Sprintf(":%d", *port))
		builder.AddAcceptor(tcp)
	}
	builder.Groups = groups.NewMemoryGroupService(*config.NewDefaultMemoryGroupConfig())
	app = builder.Build()

	//TODO: Oelze pitaya.SetSerializer(protobuf.NewSerializer())

	defer app.Shutdown()

	if !*isFrontend {
		configureBackend()
	} else {
		configureFrontend(*port)
	}

	app.Start()
}

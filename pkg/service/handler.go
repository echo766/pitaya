// Copyright (c) nano Author and TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nuid"

	"github.com/echo766/pitaya/pkg/acceptor"
	"github.com/echo766/pitaya/pkg/async"
	"github.com/echo766/pitaya/pkg/pipeline"
	"github.com/echo766/pitaya/pkg/router"

	"github.com/echo766/pitaya/pkg/agent"
	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/conn/codec"
	"github.com/echo766/pitaya/pkg/conn/message"
	"github.com/echo766/pitaya/pkg/conn/packet"
	"github.com/echo766/pitaya/pkg/constants"
	pcontext "github.com/echo766/pitaya/pkg/context"
	e "github.com/echo766/pitaya/pkg/errors"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/metrics"
	"github.com/echo766/pitaya/pkg/route"
	"github.com/echo766/pitaya/pkg/serialize"
	"github.com/echo766/pitaya/pkg/session"
	"github.com/echo766/pitaya/pkg/timer"
	"github.com/echo766/pitaya/pkg/tracing"
	opentracing "github.com/opentracing/opentracing-go"
)

var (
	handlerType = "handler"
)

type (
	// HandlerService service
	HandlerService struct {
		baseService
		decoder             codec.PacketDecoder // binary decoder
		remoteService       *RemoteService
		serializer          serialize.Serializer          // message serializer
		server              *cluster.Server               // server obj
		services            map[string]*component.Service // all registered service
		metricsReporters    []metrics.Reporter
		agentFactory        agent.AgentFactory
		handlerPool         *HandlerPool
		handlers            map[string]*component.Handler // all handler method
		concurrencyDispatch int
		clientVersion       sync.Map
		zoneId              uint32
		router              *router.Router
	}

	unhandledMessage struct {
		ctx   context.Context
		agent agent.Agent
		route *route.Route
		msg   *message.Message
	}
)

// NewHandlerService creates and returns a new handler service
func NewHandlerService(
	packetDecoder codec.PacketDecoder,
	serializer serialize.Serializer,
	localProcessBufferSize int,
	remoteProcessBufferSize int,
	server *cluster.Server,
	remoteService *RemoteService,
	agentFactory agent.AgentFactory,
	metricsReporters []metrics.Reporter,
	handlerHooks *pipeline.HandlerHooks,
	handlerPool *HandlerPool,
	threadNum int,
	zoneId uint32,
	router *router.Router,
) *HandlerService {
	h := &HandlerService{
		concurrencyDispatch: threadNum,
		services:            make(map[string]*component.Service),
		decoder:             packetDecoder,
		serializer:          serializer,
		server:              server,
		remoteService:       remoteService,
		agentFactory:        agentFactory,
		metricsReporters:    metricsReporters,
		handlerPool:         handlerPool,
		handlers:            make(map[string]*component.Handler),
		zoneId:              zoneId,
		router:              router,
	}

	h.handlerHooks = handlerHooks

	return h
}

func (h *HandlerService) SetClientVersion(v map[int]int) {
	for k, v := range v {
		h.clientVersion.Store(k, v)
	}
}

func (h *HandlerService) dispatch(msg interface{}) {
	m := msg.(*unhandledMessage)

	if m.route.SvType == h.server.Type {
		h.localProcess(m.ctx, m.agent, m.route, m.msg)
		metrics.ReportMessageProcessDelayFromCtx(m.ctx, h.metricsReporters, "local")
	} else {
		h.remoteService.remoteProcess(m.ctx, nil, m.agent, m.route, m.msg)
		metrics.ReportMessageProcessDelayFromCtx(m.ctx, h.metricsReporters, "remote")
	}
}

func (h *HandlerService) DispatchTimer() {
	// TODO: This timer is being stopped multiple times, it probably doesn't need to be stopped here
	defer timer.GlobalTicker.Stop()

	for range timer.GlobalTicker.C { // execute cron task
		timer.Cron()
	}
}

// Register registers components
func (h *HandlerService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := h.services[s.Name]; ok {
		return fmt.Errorf("handler: service already defined: %s", s.Name)
	}

	if err := s.ExtractHandler(); err != nil {
		return err
	}

	// register all handlers
	h.services[s.Name] = s
	for name, handler := range s.Handlers {
		h.handlerPool.Register(s.Name, name, handler)
	}

	h.handlerPool.SetServices(h.services)
	return nil
}

// Handle handles messages from a conn
func (h *HandlerService) Handle(conn acceptor.PlayerConn) {
	// create a client agent and startup write goroutine
	a := h.agentFactory.CreateAgent(conn)

	// startup agent goroutine
	async.GoRaw(func() {
		a.Handle(h.dispatch)
	})

	logger.Log.Debugf("New session established: %s", a.String())

	// guarantee agent related resource is destroyed
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("panic in handle message: %s", debug.Stack())
		}

		a.GetSession().Close()
		logger.Log.Debugf("Session read goroutine exit, SessionID=%d, UID=%s", a.GetSession().ID(), a.GetSession().UID())
	}()

	for {
		msg, err := conn.GetNextMessage()

		if err != nil {
			if !errors.Is(err, constants.ErrConnectionClosed) &&
				!errors.Is(err, syscall.ECONNRESET) &&
				!strings.Contains(err.Error(), "use of closed network connection") { // https://go.dev/src/net/error_test.go
				logger.Log.Errorf("Error reading next available message: %s", err.Error())
			}
			return
		}

		packets, err := h.decoder.Decode(msg)
		if err != nil {
			logger.Log.Errorf("Failed to decode message: %s", err.Error())
			return
		}

		if len(packets) < 1 {
			logger.Log.Warnf("Read no packets, data: %v", msg)
			continue
		}

		// process all packet
		for i := range packets {
			if err := h.processPacket(a, packets[i]); err != nil {
				logger.Log.Errorf("Failed to process packet: %s", err.Error())
				return
			}
		}
	}
}

func (h *HandlerService) processPacket(a agent.Agent, p *packet.Packet) error {
	switch p.Type {
	case packet.Handshake:
		logger.Log.Debug("Received handshake packet")

		// Parse the json sent with the handshake by the client
		handshakeData := &session.HandshakeData{}
		err := json.Unmarshal(p.Data, handshakeData)
		if err != nil {
			return fmt.Errorf("invalid handshake data. Id=%d", a.GetSession().ID())
		}

		needVersion, ok := h.clientVersion.Load(handshakeData.Sys.Platform)
		if !ok {
			return fmt.Errorf("invalid handshake data. client version not found. Id=%d", a.GetSession().ID())
		}

		code := 200
		if handshakeData.Sys.ResVersion < needVersion.(int) {
			code = 400
		}
		if err := a.SendHandshakeResponse(code); err != nil {
			logger.Log.Errorf("Error sending handshake response: %s", err.Error())
			return err
		}

		err = a.GetSession().Set(constants.IPVersionKey, a.IPVersion())
		if err != nil {
			logger.Log.Warnf("failed to save ip version on session: %q\n", err)
		}

		a.GetSession().SetHandshakeData(handshakeData)
		a.SetStatus(constants.StatusHandshake)
		a.SetStatus(constants.StatusWorking)

		logger.Log.Debugf("Session handshake Id=%d, Remote=%s", a.GetSession().ID(), a.RemoteAddr())

	case packet.HandshakeAck:
		a.SetStatus(constants.StatusWorking)
		logger.Log.Debugf("Receive handshake ACK Id=%d, Remote=%s", a.GetSession().ID(), a.RemoteAddr())

	case packet.Data:
		if a.GetStatus() < constants.StatusWorking {
			return fmt.Errorf("receive data on socket which is not yet ACK, session will be closed immediately, remote=%s",
				a.RemoteAddr().String())
		}

		msg, err := message.Decode(p.Data)
		if err != nil {
			return err
		}
		h.processMessage(a, msg)

	case packet.Heartbeat:
		a.OnHeartbeat()
	}

	a.SetLastAt()
	return nil
}

func (h *HandlerService) processMessage(a agent.Agent, msg *message.Message) {
	h.router.PreRouteHook(msg)

	requestID := nuid.New()
	ctx := pcontext.AddToPropagateCtx(context.Background(), constants.StartTimeKey, time.Now().UnixNano())
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RouteKey, msg.Route)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestIDKey, requestID)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.ZoneIDKey, h.zoneId)

	tags := opentracing.Tags{
		"local.id":   h.server.ID,
		"span.kind":  "server",
		"msg.type":   strings.ToLower(msg.Type.String()),
		"user.id":    a.GetSession().UID(),
		"request.id": requestID,
	}
	ctx = tracing.StartSpan(ctx, msg.Route, tags)
	ctx = context.WithValue(ctx, constants.SessionCtxKey, a.GetSession())

	r, err := route.Decode(msg.Route)
	if err != nil {
		logger.Log.WithError(err).Error("invalid route")
		a.AnswerWithError(ctx, msg.ID, e.NewError(err, e.ErrBadRequestCode))
		return
	}

	if r.SvType == "" {
		r.SvType = h.server.Type
	}
	r.Cross = msg.CrossId

	message := unhandledMessage{
		ctx:   ctx,
		agent: a,
		route: r,
		msg:   msg,
	}

	if err := a.PushUnhandleMessage(ctx, &message); err != nil {
		logger.Log.WithFields(logger.Fields{
			"route": msg.Route,
			"type":  msg.Type,
			"uid":   a.GetSession().UID(),
		}).WithError(err).Error("failed to push message")

		a.AnswerWithError(ctx, msg.ID, e.NewError(err, e.ErrInternalCode))
	}
}

func (h *HandlerService) localProcess(ctx context.Context, a agent.Agent, route *route.Route, msg *message.Message) {
	var mid uint
	switch msg.Type {
	case message.Request:
		mid = msg.ID
	case message.Notify:
		mid = 0
	}

	ret, err := h.handlerPool.ProcessHandlerMessage(ctx, route, h.serializer, h.handlerHooks, a.GetSession(), msg.Data, msg.Type, false)
	if msg.Type != message.Notify {
		if err != nil {
			logger.Log.WithFields(logger.Fields{
				"route": msg.Route,
			}).WithError(err).Error("failed to process message")

			a.AnswerWithError(ctx, mid, err)
		} else {
			err := a.GetSession().ResponseMID(ctx, mid, ret)
			if err != nil {
				tracing.FinishSpan(ctx, err)
				metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, err)
			}
		}
	} else {
		metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, nil)
		tracing.FinishSpan(ctx, err)
	}
}

// DumpServices outputs all registered services
func (h *HandlerService) DumpServices() {
	handlers := h.handlerPool.GetHandlers()
	for name := range handlers {
		logger.Log.Debugf("registered handler %s, isRawArg: %v", name, handlers[name].IsRawArg)
	}
}

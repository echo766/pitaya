//
// Copyright (c) TFG Co. All Rights Reserved.
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
	"errors"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/echo766/pitaya/pkg/actor"
	"github.com/echo766/pitaya/pkg/agent"
	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/conn/codec"
	"github.com/echo766/pitaya/pkg/conn/message"
	"github.com/echo766/pitaya/pkg/constants"
	e "github.com/echo766/pitaya/pkg/errors"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/pipeline"
	"github.com/echo766/pitaya/pkg/protos"
	"github.com/echo766/pitaya/pkg/route"
	"github.com/echo766/pitaya/pkg/router"
	"github.com/echo766/pitaya/pkg/serialize"
	"github.com/echo766/pitaya/pkg/session"
	"github.com/echo766/pitaya/pkg/tracing"
	"github.com/echo766/pitaya/pkg/util"
)

// RemoteService struct
type RemoteService struct {
	protos.UnimplementedPitayaServer

	baseService
	rpcServer              cluster.RPCServer
	serviceDiscovery       cluster.ServiceDiscovery
	serializer             serialize.Serializer
	encoder                codec.PacketEncoder
	rpcClient              cluster.RPCClient
	services               map[string]*component.Service // all registered service
	router                 *router.Router
	messageEncoder         message.Encoder
	server                 *cluster.Server // server obj
	remoteBindingListeners []cluster.RemoteBindingListener
	sessionPool            session.SessionPool
	handlerPool            *HandlerPool
	remotes                map[string]*component.Remote // all remote method
}

// NewRemoteService creates and return a new RemoteService
func NewRemoteService(
	rpcClient cluster.RPCClient,
	rpcServer cluster.RPCServer,
	sd cluster.ServiceDiscovery,
	encoder codec.PacketEncoder,
	serializer serialize.Serializer,
	router *router.Router,
	messageEncoder message.Encoder,
	server *cluster.Server,
	sessionPool session.SessionPool,
	handlerHooks *pipeline.HandlerHooks,
	handlerPool *HandlerPool,
) *RemoteService {
	remote := &RemoteService{
		services:               make(map[string]*component.Service),
		rpcClient:              rpcClient,
		rpcServer:              rpcServer,
		encoder:                encoder,
		serviceDiscovery:       sd,
		serializer:             serializer,
		router:                 router,
		messageEncoder:         messageEncoder,
		server:                 server,
		remoteBindingListeners: make([]cluster.RemoteBindingListener, 0),
		sessionPool:            sessionPool,
		handlerPool:            handlerPool,
		remotes:                make(map[string]*component.Remote),
	}

	remote.handlerHooks = handlerHooks

	return remote
}

func (r *RemoteService) remoteProcess(
	ctx context.Context,
	server *cluster.Server,
	a agent.Agent,
	route *route.Route,
	msg *message.Message,
) {
	res, err := r.remoteCall(ctx, server, protos.RPCType_Sys, route, a.GetSession(), msg)
	switch msg.Type {
	case message.Request:
		if err != nil {
			logger.Log.WithField("route", route.String()).WithError(err).Error("error making remote call")
			a.AnswerWithError(ctx, msg.ID, err)
			return
		}
		err := a.GetSession().ResponseMID(ctx, msg.ID, res.Data)
		if err != nil {
			logger.Log.WithField("route", route.String()).WithError(err).Error("error sending response")
			a.AnswerWithError(ctx, msg.ID, err)
		}
	case message.Notify:
		defer tracing.FinishSpan(ctx, err)
		if err == nil && res.Error != nil {
			err = errors.New(res.Error.GetMsg())
		}
		if err != nil {
			logger.Log.WithField("route", route.String()).WithError(err).Error("error making remote call")
		}
	}
}

// AddRemoteBindingListener adds a listener
func (r *RemoteService) AddRemoteBindingListener(bindingListener cluster.RemoteBindingListener) {
	r.remoteBindingListeners = append(r.remoteBindingListeners, bindingListener)
}

// Call processes a remote call
func (r *RemoteService) Call(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	c, err := util.GetContextFromRequest(req, r.server.ID)

	c = util.StartSpanFromRequest(c, r.server.ID, req.GetMsg().GetRoute())
	defer tracing.FinishSpan(c, err)

	var res *protos.Response
	if err != nil {
		res = &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
	} else {
		res = r.processRemoteMessage(c, req)
	}

	if res.Error != nil {
		err = errors.New(res.Error.Msg)
	}

	return res, err
}

// SessionBindRemote is called when a remote server binds a user session and want us to acknowledge it
func (r *RemoteService) SessionBindRemote(ctx context.Context, msg *protos.BindMsg) (*protos.Response, error) {
	for _, r := range r.remoteBindingListeners {
		r.OnUserBind(msg.Uid, msg.Fid)
	}
	return &protos.Response{
		Data: []byte("ack"),
	}, nil
}

// PushToUser sends a push to user
func (r *RemoteService) PushToUser(ctx context.Context, push *protos.Push) (*protos.Response, error) {
	logger.Log.Debugf("sending push to user %s: %v", push.GetUid(), string(push.Data))
	s := r.sessionPool.GetSessionByUID(push.GetUid())
	if s != nil {
		err := s.Push(push.Route, push.Data)
		if err != nil {
			return nil, err
		}
		return &protos.Response{
			Data: []byte("ack"),
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// KickUser sends a kick to user
func (r *RemoteService) KickUser(ctx context.Context, kick *protos.KickMsg) (*protos.KickAnswer, error) {
	logger.Log.Debugf("sending kick to user %s", kick.GetUserId())
	s := r.sessionPool.GetSessionByUID(kick.GetUserId())
	if s != nil {
		err := s.Kick(ctx)
		if err != nil {
			return nil, err
		}
		return &protos.KickAnswer{
			Kicked: true,
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// DoRPC do rpc and get answer
func (r *RemoteService) DoRPC(ctx context.Context, serverID string, route *route.Route, protoData []byte) (*protos.Response, error) {
	msg := &message.Message{
		Type:  message.Request,
		Route: route.Short(),
		Data:  protoData,
	}

	if serverID == "" {
		return r.remoteCall(ctx, nil, protos.RPCType_User, route, nil, msg)
	}

	target, _ := r.serviceDiscovery.GetServer(serverID)
	if target == nil {
		return nil, constants.ErrServerNotFound
	}

	return r.remoteCall(ctx, target, protos.RPCType_User, route, nil, msg)
}

// RPC makes rpcs
func (r *RemoteService) RPC(ctx context.Context, serverID string, route *route.Route, reply proto.Message, arg proto.Message) error {
	var data []byte
	var err error
	if arg != nil {
		data, err = proto.Marshal(arg)
		if err != nil {
			return err
		}
	}
	res, err := r.DoRPC(ctx, serverID, route, data)
	if err != nil {
		return err
	}

	if res.Error != nil {
		return &e.Error{
			Code:     res.Error.Code,
			Message:  res.Error.Msg,
			Metadata: res.Error.Metadata,
		}
	}

	if reply != nil {
		err = proto.Unmarshal(res.GetData(), reply)
		if err != nil {
			return err
		}
	}
	return nil
}

// Register registers components
func (r *RemoteService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := r.services[s.Name]; ok {
		return fmt.Errorf("remote: service already defined: %s", s.Name)
	}

	if err := s.ExtractRemote(); err != nil {
		return err
	}

	r.services[s.Name] = s
	// register all remotes
	for name, remote := range s.Remotes {
		r.remotes[fmt.Sprintf("%s.%s", s.Name, name)] = remote
	}

	return nil
}

func (r *RemoteService) ExtendRemote(svc, name string, remote *component.Remote) error {
	s, ok := r.services[svc]
	if !ok {
		return fmt.Errorf("remote: service not found: %s", svc)
	}

	if _, ok := s.Remotes[name]; ok {
		return fmt.Errorf("remote: remote already defined: %s.%s", svc, name)
	}

	s.Remotes[name] = remote
	r.remotes[fmt.Sprintf("%s.%s", svc, name)] = remote
	return nil
}

func (r *RemoteService) processRemoteMessage(ctx context.Context, req *protos.Request) *protos.Response {
	rt, err := route.Decode(req.GetMsg().GetRoute())
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrBadRequestCode,
				Msg:  "cannot decode route",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
		return response
	}

	switch {
	case req.Type == protos.RPCType_Sys:
		return r.handleRPCSys(ctx, req, rt)
	case req.Type == protos.RPCType_User:
		return r.handleRPCUser(ctx, req, rt)
	default:
		return &protos.Response{
			Error: &protos.Error{
				Code: e.ErrBadRequestCode,
				Msg:  "invalid rpc type",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
	}
}

func (r *RemoteService) RpcInner(ctx context.Context, reply proto.Message, arg proto.Message, rt *route.Route) error {
	var err error
	ctx = util.StartSpanFromRequest(ctx, r.server.ID, rt.String())
	defer tracing.FinishSpan(ctx, err)

	ac, remote := r.getRemote(ctx, rt)
	if ac == nil || remote == nil {
		return constants.ErrNoRemoteHandler
	}

	params := []reflect.Value{remote.Receiver, reflect.ValueOf(ctx)}
	if remote.HasArgs {
		params = append(params, reflect.ValueOf(arg))
	}

	ctx = context.WithValue(ctx, constants.RouteCtxKey, rt)

	if ah, ok := ac.(interface {
		BeforeRemoteHandle(
			ctx context.Context,
			rt *route.Route,
			hd *component.Remote,
			arg interface{}) (actor.Actor, *component.Remote, error)
	}); ok {
		var err error
		if ac, remote, err = ah.BeforeRemoteHandle(ctx, rt, remote, params[2].Interface()); err != nil {
			return err
		}
		params[0] = remote.Receiver
	}

	ret, err := ac.Exec(ctx, func() (interface{}, error) {
		return util.Call(remote.Method, params)
	})

	if ret == nil || err != nil {
		return err
	}

	pb, ok := ret.(proto.Message)
	if !ok {
		return fmt.Errorf("remote: invalid return type %T", ret)
	}

	if reply != nil {
		data, err := proto.Marshal(pb)
		if err != nil {
			return err
		}
		return proto.Unmarshal(data, reply)
	}
	return nil
}

func (h *RemoteService) getActor(ctx context.Context, rt *route.Route) (actor.Actor, error) {
	svc, ok := h.services[rt.Service]
	if !ok {
		return nil, constants.ErrInvalidRoute
	}
	ac, ok := svc.Receiver.Interface().(actor.Actor)
	if !ok {
		return nil, constants.ErrInvalidRoute
	}
	if !rt.HasSub() {
		return ac, nil
	}
	sac, ok := ac.(interface {
		GetSubActor(ctx context.Context) (actor.Actor, error)
	})
	if !ok {
		return ac, nil
	}
	return sac.GetSubActor(ctx)
}

func (r *RemoteService) getRemote(ctx context.Context, rt *route.Route) (actor.Actor, *component.Remote) {
	remote, ok := r.remotes[rt.Short()]
	if ok {
		ac, ok := remote.Receiver.Interface().(actor.Actor)
		if !ok {
			logger.Log.Warnf("pitaya/remote: remote isnot actor. %s", rt.String())
			return nil, nil
		}

		return ac, remote
	}

	ac, err := r.getActor(ctx, rt)
	if err != nil {
		logger.Log.Warnf("pitaya/remote: %s not found", rt.Short())
		return nil, nil
	}

	ah, ok := ac.(interface {
		GetHandler(ctx context.Context, rt *route.Route) (*component.Handler, error)
	})
	if !ok {
		return nil, nil
	}

	handler, err := ah.GetHandler(ctx, rt)
	if err != nil {
		return nil, nil
	}

	remote = &component.Remote{
		Receiver: handler.Receiver,
		Method:   handler.Method,
		HasArgs:  true,
		Type:     handler.Type,
	}

	return ac, remote
}

func (r *RemoteService) handleRPCUser(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	ac, remote := r.getRemote(ctx, rt)
	if ac == nil || remote == nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrNotFoundCode,
				Msg:  "remote: no handler found",
				Metadata: map[string]string{
					"route": rt.Short(),
				},
			},
		}
		return response
	}

	params := []reflect.Value{remote.Receiver, reflect.ValueOf(ctx)}
	if remote.HasArgs {
		arg, err := unmarshalRemoteArg(remote, req.GetMsg().GetData())
		if err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrBadRequestCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
		params = append(params, reflect.ValueOf(arg))
	}

	ctx = context.WithValue(ctx, constants.RouteCtxKey, rt)

	if ah, ok := ac.(interface {
		BeforeRemoteHandle(
			ctx context.Context,
			rt *route.Route,
			hd *component.Remote,
			arg interface{}) (actor.Actor, *component.Remote, error)
	}); ok {
		var err error
		if ac, remote, err = ah.BeforeRemoteHandle(ctx, rt, remote, params[2].Interface()); err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrInternalCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
		params[0] = remote.Receiver
	}

	ret, err := ac.Exec(ctx, func() (interface{}, error) {
		return util.Call(remote.Method, params)
	})

	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrUnknownCode,
				Msg:  err.Error(),
			},
		}
		if val, ok := err.(*e.Error); ok {
			response.Error.Code = val.Code
			if val.Metadata != nil {
				response.Error.Metadata = val.Metadata
			}
		}
		return response
	}

	var b []byte
	if ret != nil {
		pb, ok := ret.(proto.Message)
		if !ok {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  constants.ErrWrongValueType.Error(),
				},
			}
			return response
		}
		if b, err = proto.Marshal(pb); err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
	}

	return &protos.Response{Data: b}
}

func (r *RemoteService) handleRPCSys(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	reply := req.GetMsg().GetReply()
	response := &protos.Response{}
	// (warning) a new agent is created for every new request
	a, err := agent.NewRemote(
		req.GetSession(),
		reply,
		r.rpcClient,
		r.encoder,
		r.serializer,
		r.serviceDiscovery,
		req.FrontendID,
		r.messageEncoder,
		r.sessionPool,
	)
	if err != nil {
		logger.Log.WithFields(logger.Fields{
			"router": rt.String(),
		}).WithError(err).Error("error creating agent")

		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
		return response
	}

	ret, err := r.handlerPool.ProcessHandlerMessage(ctx, rt, r.serializer, r.handlerHooks, a.Session, req.GetMsg().GetData(), req.GetMsg().GetType(), true)
	if err != nil {
		logger.Log.WithFields(logger.Fields{
			"router": rt.String(),
		}).WithError(err).Error("error handling request")

		response = &protos.Response{
			Error: &protos.Error{
				Code: e.ErrUnknownCode,
				Msg:  err.Error(),
			},
		}
		if val, ok := err.(*e.Error); ok {
			response.Error.Code = val.Code
			if val.Metadata != nil {
				response.Error.Metadata = val.Metadata
			}
		}
	} else {
		response = &protos.Response{Data: ret}
	}
	return response
}

func (r *RemoteService) remoteCall(
	ctx context.Context,
	server *cluster.Server,
	rpcType protos.RPCType,
	route *route.Route,
	session session.Session,
	msg *message.Message,
) (*protos.Response, error) {
	svType := route.SvType

	var err error
	target := server

	if target == nil {
		for i := 0; i < 30; i++ {
			target, err = r.router.Route(ctx, rpcType, svType, route, msg)
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			return nil, e.NewError(err, e.ErrInternalCode)
		}
	}

	res, err := r.rpcClient.Call(ctx, rpcType, route, session, msg, target)
	if err != nil {
		logger.Log.WithFields(logger.Fields{
			"id":    target.ID,
			"route": route.String(),
			"host":  target.Hostname,
		}).WithError(err).Error("error making call to target")
		return nil, err
	}
	return res, err
}

// DumpServices outputs all registered services
func (r *RemoteService) DumpServices() {
	for name := range r.remotes {
		logger.Log.Debugf("registered remote %s", name)
	}
}

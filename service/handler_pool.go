package service

import (
	"context"
	"fmt"
	"reflect"

	"github.com/echo766/pitaya/actor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/conn/message"
	"github.com/echo766/pitaya/constants"
	e "github.com/echo766/pitaya/errors"
	"github.com/echo766/pitaya/logger/interfaces"
	"github.com/echo766/pitaya/pipeline"
	"github.com/echo766/pitaya/route"
	"github.com/echo766/pitaya/serialize"
	"github.com/echo766/pitaya/session"
	"github.com/echo766/pitaya/util"
)

// HandlerPool ...
type HandlerPool struct {
	handlers map[string]*component.Handler // all handler method
	services map[string]*component.Service
}

// NewHandlerPool ...
func NewHandlerPool() *HandlerPool {
	return &HandlerPool{
		handlers: make(map[string]*component.Handler),
		services: make(map[string]*component.Service),
	}
}

// Register ...
func (h *HandlerPool) Register(serviceName string, name string, handler *component.Handler) {
	h.handlers[fmt.Sprintf("%s.%s", serviceName, name)] = handler
}

// GetHandlers ...
func (h *HandlerPool) GetHandlers() map[string]*component.Handler {
	return h.handlers
}

func (h *HandlerPool) SetServices(services map[string]*component.Service) {
	h.services = services
}

func (h *HandlerPool) getActor(ctx context.Context, rt *route.Route) (actor.Actor, error) {
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
		return nil, constants.ErrInvalidRoute
	}
	return sac.GetSubActor(ctx)
}

func (h *HandlerPool) actorProcess(
	ctx context.Context,
	ac actor.Actor,
	rt *route.Route,
	serializer serialize.Serializer,
	handlerHooks *pipeline.HandlerHooks,
	session session.Session,
	data []byte,
	msgTypeIface interface{},
	remote bool,
) (interface{}, error) {
	handler, err := h.getHandler(rt)
	if err != nil {
		ah, ok := ac.(interface {
			GetHandler(ctx context.Context, rt *route.Route) (*component.Handler, error)
		})
		if !ok {
			return nil, e.NewError(err, e.ErrNotFoundCode)
		}
		handler, err = ah.GetHandler(ctx, rt)
		if err != nil {
			return nil, e.NewError(err, e.ErrNotFoundCode)
		}
	}

	msgType, err := getMsgType(msgTypeIface)
	if err != nil {
		return nil, e.NewError(err, e.ErrInternalCode)
	}

	logger := ctx.Value(constants.LoggerCtxKey).(interfaces.Logger)
	exit, err := handler.ValidateMessageType(msgType)
	if err != nil && exit {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	} else if err != nil {
		logger.Warnf("invalid message type, error: %s", err.Error())
	}

	// First unmarshal the handler arg that will be passed to
	// both handler and pipeline functions
	arg, err := unmarshalHandlerArg(handler, serializer, data)
	if err != nil {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	}

	ctx, arg, err = handlerHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
	if err != nil {
		return nil, err
	}

	logger.Debugf("SID=%d, Data=%s", session.ID(), data)
	args := []reflect.Value{handler.Receiver, reflect.ValueOf(ctx)}
	if arg != nil {
		args = append(args, reflect.ValueOf(arg))
	}

	resp, err := util.Pcall(handler.Method, args)
	if remote && msgType == message.Notify {
		// This is a special case and should only happen with nats rpc client
		// because we used nats request we have to answer to it or else a timeout
		// will happen in the caller server and will be returned to the client
		// the reason why we don't just Publish is to keep track of failed rpc requests
		// with timeouts, maybe we can improve this flow
		resp = []byte("ack")
	}

	return handlerHooks.AfterHandler.ExecuteAfterPipeline(ctx, resp, err)
}

// ProcessHandlerMessage ...
func (h *HandlerPool) ProcessHandlerMessage(
	ctx context.Context,
	rt *route.Route,
	serializer serialize.Serializer,
	handlerHooks *pipeline.HandlerHooks,
	session session.Session,
	data []byte,
	msgTypeIface interface{},
	remote bool,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, constants.SessionCtxKey, session)
	ctx = context.WithValue(ctx, constants.RouteCtxKey, rt)
	ctx = util.CtxWithDefaultLogger(ctx, rt.String(), session.UID())

	ac, err := h.getActor(ctx, rt)
	if err != nil {
		return nil, e.NewError(err, e.ErrNotFoundCode)
	}

	resp, err := ac.Exec(ctx, func() (interface{}, error) {
		return h.actorProcess(ctx, ac, rt, serializer, handlerHooks, session, data, msgTypeIface, remote)
	})
	if err != nil {
		return nil, err
	}

	ret, err := serializeReturn(serializer, resp)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (h *HandlerPool) getHandler(rt *route.Route) (*component.Handler, error) {
	handler, ok := h.handlers[rt.Short()]
	if !ok {
		e := fmt.Errorf("pitaya/handler: %s not found", rt.String())
		return nil, e
	}
	return handler, nil

}

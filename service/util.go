// Copyright (c) TFG Co. All Rights Reserved.
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
	"errors"
	"fmt"
	"reflect"

	"github.com/echo766/pitaya/actor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/conn/message"
	"github.com/echo766/pitaya/constants"
	e "github.com/echo766/pitaya/errors"
	"github.com/echo766/pitaya/logger"
	"github.com/echo766/pitaya/pipeline"
	"github.com/echo766/pitaya/protos"
	"github.com/echo766/pitaya/route"
	"github.com/echo766/pitaya/serialize"
	"github.com/echo766/pitaya/session"
	"github.com/echo766/pitaya/util"
	"github.com/golang/protobuf/proto"
)

var errInvalidMsg = errors.New("invalid message type provided")

func getHandler(ctx context.Context, rt *route.Route) (*component.Handler, error) {
	handler, ok := handlers[rt.Short()]
	if !ok {
		return nil, fmt.Errorf("pitaya/handler: %s not found", rt.String())
	}

	return handler, nil
}

func getHandleActor(ctx context.Context, rt *route.Route) (actor.Actor, error) {
	svc, ok := services[rt.Service]
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

func unmarshalHandlerArg(handler *component.Handler, serializer serialize.Serializer, payload []byte) (interface{}, error) {
	if handler.IsRawArg {
		return payload, nil
	}

	var arg interface{}
	if handler.Type != nil {
		arg = reflect.New(handler.Type.Elem()).Interface()
		err := serializer.Unmarshal(payload, arg)
		if err != nil {
			return nil, err
		}
	}
	return arg, nil
}

func unmarshalRemoteArg(remote *component.Remote, payload []byte) (interface{}, error) {
	var arg interface{}
	if remote.Type != nil {
		arg = reflect.New(remote.Type.Elem()).Interface()
		pb, ok := arg.(proto.Message)
		if !ok {
			return nil, constants.ErrWrongValueType
		}
		err := proto.Unmarshal(payload, pb)
		if err != nil {
			return nil, err
		}
	}
	return arg, nil
}

func getMsgType(msgTypeIface interface{}) (message.Type, error) {
	var msgType message.Type
	if val, ok := msgTypeIface.(message.Type); ok {
		msgType = val
	} else if val, ok := msgTypeIface.(protos.MsgType); ok {
		msgType = util.ConvertProtoToMessageType(val)
	} else {
		return msgType, errInvalidMsg
	}
	return msgType, nil
}

func executeBeforePipeline(ctx context.Context, data interface{}) (context.Context, interface{}, error) {
	var err error
	res := data
	if len(pipeline.BeforeHandler.Handlers) > 0 {
		for _, h := range pipeline.BeforeHandler.Handlers {
			ctx, res, err = h(ctx, res)
			if err != nil {
				logger.Log.Debugf("pitaya/handler: broken pipeline: %s", err.Error())
				return ctx, res, err
			}
		}
	}
	return ctx, res, nil
}

func executeAfterPipeline(ctx context.Context, res interface{}, err error) (interface{}, error) {
	ret := res
	if len(pipeline.AfterHandler.Handlers) > 0 {
		for _, h := range pipeline.AfterHandler.Handlers {
			ret, err = h(ctx, ret, err)
		}
	}
	return ret, err
}

func serializeReturn(ser serialize.Serializer, ret interface{}) ([]byte, error) {
	res, err := util.SerializeOrRaw(ser, ret)
	if err != nil {
		logger.Log.Errorf("Failed to serialize return: %s", err.Error())
		res, err = util.GetErrorPayload(ser, err)
		if err != nil {
			logger.Log.Error("cannot serialize message and respond to the client ", err.Error())
			return nil, err
		}
	}
	return res, nil
}

// execute on actor routine
func asnycProcessMessage(
	ctx context.Context,
	ac actor.Actor,
	rt *route.Route,
	serializer serialize.Serializer,
	session *session.Session,
	data []byte,
	msgTypeIface interface{},
	remote bool,
) (interface{}, error) {
	h, err := getHandler(ctx, rt)
	if err != nil {
		ah, ok := ac.(interface {
			GetHandler(ctx context.Context, rt *route.Route) (*component.Handler, error)
		})
		if !ok {
			return nil, e.NewError(err, e.ErrBadRequestCode)
		}
		h, err = ah.GetHandler(ctx, rt)
		if err != nil {
			return nil, e.NewError(err, e.ErrBadRequestCode)
		}
	}

	msgType, err := getMsgType(msgTypeIface)
	if err != nil {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	}

	logger := ctx.Value(constants.LoggerCtxKey).(logger.Logger)
	exit, err := h.ValidateMessageType(msgType)
	if err != nil && exit {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	} else if err != nil {
		logger.Warnf("invalid message type, error: %s", err.Error())
	}

	// First unmarshal the handler arg that will be passed to
	// both handler and pipeline functions
	arg, err := unmarshalHandlerArg(h, serializer, data)
	if err != nil {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	}

	ctx, arg, err = executeBeforePipeline(ctx, arg)
	if err != nil {
		return nil, e.NewError(err, e.ErrBadRequestCode)
	}

	logger.Debugf("SID=%d, Data=%s", session.ID(), data)
	args := []reflect.Value{h.Receiver, reflect.ValueOf(ctx)}
	if arg != nil {
		args = append(args, reflect.ValueOf(arg))
	}

	resp, err := util.Pcall(h.Method, args)
	if err != nil {
		return nil, e.NewError(err, e.ErrInternalCode)
	}

	if remote && msgType == message.Notify {
		// This is a special case and should only happen with nats rpc client
		// because we used nats request we have to answer to it or else a timeout
		// will happen in the caller server and will be returned to the client
		// the reason why we don't just Publish is to keep track of failed rpc requests
		// with timeouts, maybe we can improve this flow
		resp = []byte("ack")
	}

	resp, err = executeAfterPipeline(ctx, resp, err)
	if err != nil {
		return nil, e.NewError(err, e.ErrInternalCode)
	}
	return resp, nil
}

func processHandlerMessage(
	ctx context.Context,
	rt *route.Route,
	serializer serialize.Serializer,
	session *session.Session,
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

	ac, err := getHandleActor(ctx, rt)
	if err != nil {
		return nil, e.NewError(err, e.ErrNotFoundCode)
	}

	resp, err := ac.Exec(ctx, func() (interface{}, error) {
		return asnycProcessMessage(ctx, ac, rt, serializer, session, data, msgTypeIface, remote)
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

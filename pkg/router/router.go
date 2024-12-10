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

package router

import (
	"context"

	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/conn/message"
	"github.com/echo766/pitaya/pkg/constants"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/protos"
	"github.com/echo766/pitaya/pkg/route"
)

// Router struct
type Router struct {
	routesMap     map[string]RoutingFunc
	preRouteHooks []PreRouteHookFunc
}

type (
	// RoutingFunc defines a routing function
	RoutingFunc func(
		ctx context.Context,
		route *route.Route,
		payload []byte,
		rpcType protos.RPCType,
	) (*cluster.Server, error)

	PreRouteHookFunc func(msg *message.Message)
)

// New returns the router
func New() *Router {
	return &Router{
		routesMap: make(map[string]RoutingFunc),
	}
}

func (r *Router) AddPreRouteHook(hook PreRouteHookFunc) {
	r.preRouteHooks = append(r.preRouteHooks, hook)
}

func (r *Router) PreRouteHook(
	msg *message.Message,
) {
	for _, hook := range r.preRouteHooks {
		hook(msg)
	}
}

// Route gets the right server to use in the call
func (r *Router) Route(
	ctx context.Context,
	rpcType protos.RPCType,
	svType string,
	route *route.Route,
	msg *message.Message,
) (*cluster.Server, error) {
	routeFunc, ok := r.routesMap[svType]
	if !ok {
		logger.Log.Errorf("no specific route for svType: %s, using default route", svType)
		return nil, constants.ErrNoServerTypeChosenForRPC
	}
	return routeFunc(ctx, route, msg.Data, rpcType)
}

// AddRoute adds a routing function to a server type
func (r *Router) AddRoute(
	serverType string,
	routingFunction RoutingFunc,
) {
	if _, ok := r.routesMap[serverType]; ok {
		logger.Log.Warnf("overriding the route to svType %s", serverType)
	}
	r.routesMap[serverType] = routingFunction
}

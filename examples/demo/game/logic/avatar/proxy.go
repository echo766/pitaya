package avatar

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/actor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/route"
)

type (
	AvatarProxy struct {
		actor.Impl
		avatar *Avatar

		services map[string]*component.Service
		handlers map[string]*component.Handler
		evts     map[string][]*component.Handler
	}
)

func NewAvatarProxy() *AvatarProxy {
	return &AvatarProxy{}
}

func (a *AvatarProxy) Init() {
	a.Impl.Init()
	a.reset()
}

func (a *AvatarProxy) reset() {
	a.avatar = nil
	a.services = make(map[string]*component.Service)
	a.handlers = make(map[string]*component.Handler)
	a.evts = make(map[string][]*component.Handler)
}

func (a *AvatarProxy) registerHandler(comp interface{}, opts ...component.Option) error {
	s := component.NewService(comp, opts)
	a.services[s.Name] = s

	if err := s.ExtractHandler(); err == nil {
		// register all handlers
		for name, handler := range s.Handlers {
			a.handlers[fmt.Sprintf("%s.%s", s.Name, name)] = handler
		}
	}
	if err := s.ExtractEvent(); err == nil {
		for name, handler := range s.Events {
			a.evts[name] = append(a.evts[name], handler)
		}
	}
	return nil
}

func (a *AvatarProxy) createEntity(uid int64) error {
	a.avatar = NewAvatar()
	a.avatar.Init(uid)

	a.registerHandler(a.avatar, component.WithName("avatar"),
		component.WithNameFunc(strings.ToLower))

	for _, comp := range a.avatar.Comps() {
		a.registerHandler(comp, component.WithName(fmt.Sprintf("avatar.%s", comp.Name())),
			component.WithNameFunc(strings.ToLower))
	}

	a.avatar.RegistEvent(a.evts)
	return nil
}

func (a *AvatarProxy) preExec(ctx context.Context) error {
	if a.avatar != nil {
		return nil
	}

	session := pitaya.GetSessionFromCtx(ctx)
	if session == nil {
		return fmt.Errorf("avatar proxy pre exec not found session")
	}
	rt := pitaya.GetRouteFromCtx(ctx)
	if rt == nil {
		return fmt.Errorf("avatar proxy pre exec not found route")
	}
	if !rt.HasSub() || rt.SubShort() != "avatar.login" {
		return fmt.Errorf("avatar is not logged in")
	}

	uid, err := strconv.ParseInt(session.UID(), 10, 64)
	if err != nil {
		return fmt.Errorf("avatar proxy pre exec uid error. uid %v", uid)
	}

	return a.createEntity(uid)
}

func (a *AvatarProxy) tryReleaseEnt() {
	if a.avatar != nil && a.avatar.IsAFK() {
		a.reset()
	}
}

func (a *AvatarProxy) afterExec(ctx context.Context, resp interface{}, ret error) (interface{}, error) {
	// TODO: 将返回值直接发送给客户端
	return resp, ret
}

func (a *AvatarProxy) Exec(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	return a.Impl.Exec(ctx, func() (interface{}, error) {
		defer a.tryReleaseEnt()

		if err := a.preExec(ctx); err != nil {
			return nil, err
		}

		resp, ret := fn()
		return a.afterExec(ctx, resp, ret)
	})
}

func (a *AvatarProxy) Push(ctx context.Context, fn func() (interface{}, error)) error {
	return a.Impl.Push(ctx, func() (interface{}, error) {
		defer a.tryReleaseEnt()

		if err := a.preExec(ctx); err != nil {
			return nil, err
		}

		resp, ret := fn()
		return a.afterExec(ctx, resp, ret)
	})
}

func (a *AvatarProxy) GetHandler(ctx context.Context, rt *route.Route) (*component.Handler, error) {
	handler, ok := a.handlers[rt.SubShort()]
	if !ok {
		return nil, fmt.Errorf("pitaya/avatar/handler: %s not found", rt.String())
	}

	return handler, nil
}

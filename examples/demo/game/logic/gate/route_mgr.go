package gate

import (
	"context"
	"sync"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/cluster"
	"github.com/echo766/pitaya/constants"
	"github.com/echo766/pitaya/logger"
	"github.com/echo766/pitaya/modules"
	"github.com/echo766/pitaya/route"
)

type (
	RouteMgr struct {
		lock *sync.RWMutex
		modules.Base

		svrs map[string]map[string]*cluster.Server
	}
)

func NewRouteMgr() *RouteMgr {
	return &RouteMgr{}
}

func (mgr *RouteMgr) Init() error {
	mgr.lock = &sync.RWMutex{}
	mgr.svrs = make(map[string]map[string]*cluster.Server)
	return nil
}

func (mgr *RouteMgr) AddServer(svr *cluster.Server) {
	id, ok := svr.Metadata[constants.ServerIDKey]
	if !ok {
		logger.Log.Errorf("add server error. server id is not set. id: %v type: %v", svr.ID, svr.Type)
		return
	}
	if id != pitaya.GetServerID() {
		return
	}

	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	if _, ok := mgr.svrs[svr.Type]; !ok {
		mgr.svrs[svr.Type] = make(map[string]*cluster.Server)
	}
	mgr.svrs[svr.Type][svr.ID] = svr
}

func (mgr *RouteMgr) RemoveServer(svr *cluster.Server) {
	id, ok := svr.Metadata[constants.ServerIDKey]
	if !ok {
		logger.Log.Errorf("add server error. server id is not set. id: %v type: %v", svr.ID, svr.Type)
		return
	}
	if id != pitaya.GetServerID() {
		return
	}

	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	if _, ok := mgr.svrs[svr.Type]; !ok {
		return
	}
	delete(mgr.svrs[svr.Type], svr.ID)
}

func (mgr *RouteMgr) route(svrType string) *cluster.Server {
	svrs, ok := mgr.svrs[svrType]
	if !ok || len(svrs) == 0 {
		return nil
	}

	for _, svr := range svrs {
		return svr
	}
	return nil
}

func (mgr *RouteMgr) routeAS() *cluster.Server {
	// avatar server 路由选择
	// 后续可以考虑支持多个avatar server以及avatar server滚动更新
	return mgr.route("avatar")
}

func (mgr *RouteMgr) Route(ctx context.Context, rt *route.Route) (*cluster.Server, error) {
	session := pitaya.GetSessionFromCtx(ctx)
	if session.UID() == "" {
		rs := rt.String()
		if rs != "logic.loginmgr.login" && rs != "avatar.avatarmgr.relogin" {
			return nil, constants.ErrPlayerNotLogin
		}
	}

	mgr.lock.RLock()
	defer mgr.lock.RUnlock()

	// 第一次路由首先设置avatar server
	as := session.Get("avatar_server")
	if as == nil {
		as := mgr.routeAS()
		if as == nil {
			return nil, constants.ErrNoServerTypeChosenForRPC
		}
	}

	if rt.SvType == "avatar" {
		return as.(*cluster.Server), nil
	}

	svr := mgr.route(rt.SvType)
	if svr == nil {
		return nil, constants.ErrNoServerTypeChosenForRPC
	}

	return svr, nil
}

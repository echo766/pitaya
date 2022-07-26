package avatar

import (
	"context"
	"strconv"
	"sync"

	"github.com/echo766/pitaya"
	"github.com/echo766/pitaya/actor"
	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/constants"
)

type AvatarMgr struct {
	lock *sync.Mutex
	component.Base
	actors actor.Group
}

func NewAvatarMgr() *AvatarMgr {
	return &AvatarMgr{}
}

func (mgr *AvatarMgr) Init() {
	mgr.Base.Init()
	mgr.lock = &sync.Mutex{}
	mgr.actors = actor.NewActorGroup(mgr)
}

func (mgr *AvatarMgr) GetSubActor(ctx context.Context) (actor.Actor, error) {
	session := pitaya.GetSessionFromCtx(ctx)
	if session == nil {
		return nil, constants.ErrSessionNotFound
	}

	uid, err := strconv.ParseInt(session.UID(), 10, 64)
	if err != nil {
		return nil, constants.ErrIllegalUID
	}

	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	proxy := mgr.actors.GetActor(uid)
	if proxy == nil {
		proxy = NewAvatarProxy()
		proxy.Init()
		proxy.Start()
		mgr.actors.AddActor(uid, proxy)
	}

	return proxy, nil
}

func (mgr *AvatarMgr) Bind(ctx context.Context, msg []byte) {
	s := pitaya.GetSessionFromCtx(ctx)
	fakeUID := s.ID()                       // just use s.ID as uid !!!
	s.Bind(ctx, strconv.Itoa(int(fakeUID))) // binding session uid
}

package actor

import "sync"

type (
	Group interface {
		GetActor(Id int64) Actor
		AddActor(Id int64, ac Actor)
		DelActor(Id int64)
		GetActorNum() int
		GetOwner() Actor
	}

	groupImpl struct {
		lock   *sync.RWMutex
		owner  Actor
		actors map[int64]Actor
	}
)

func NewActorGroup(owner Actor) Group {
	g := groupImpl{}
	g.Init(owner)
	return &g
}

func (v *groupImpl) Init(owner Actor) {
	v.owner = owner
	v.lock = new(sync.RWMutex)
	v.actors = make(map[int64]Actor)
}

func (v *groupImpl) GetActor(Id int64) Actor {
	v.lock.RLock()
	defer v.lock.RUnlock()

	ac, ok := v.actors[Id]
	if !ok {
		return nil
	}
	return ac
}

func (v *groupImpl) AddActor(Id int64, ac Actor) {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.actors[Id] = ac
}

func (v *groupImpl) DelActor(Id int64) {
	v.lock.Lock()
	defer v.lock.Unlock()

	delete(v.actors, Id)
}

func (v *groupImpl) GetActorNum() int {
	v.lock.RLock()
	defer v.lock.RUnlock()

	return len(v.actors)
}

func (v *groupImpl) GetOwner() Actor {
	return v.owner
}

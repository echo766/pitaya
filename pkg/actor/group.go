package actor

import "sync"

type (
	Group interface {
		GetActor(Id uint64) Actor
		AddActor(Id uint64, ac Actor)
		DelActor(Id uint64)
		GetOwner() Actor
		Each(func(uint64, Actor))
		Stop()
		Wait()
		Count() int
	}

	groupImpl struct {
		owner  Actor
		actors sync.Map
	}
)

func NewGroup(owner Actor) Group {
	g := groupImpl{
		owner: owner,
	}
	return &g
}

func NewSafeGroup(owner Actor) Group {
	g := groupImpl{
		owner: owner,
	}
	return &g
}

func (gi *groupImpl) GetActor(Id uint64) Actor {
	ac, ok := gi.actors.Load(Id)
	if !ok {
		return nil
	}
	return ac.(Actor)
}

func (gi *groupImpl) AddActor(Id uint64, ac Actor) {
	gi.actors.Store(Id, ac)
}

func (gi *groupImpl) DelActor(Id uint64) {
	gi.actors.Delete(Id)
}

func (gi *groupImpl) Each(f func(uint64, Actor)) {
	gi.actors.Range(func(key, value interface{}) bool {
		f(key.(uint64), value.(Actor))
		return true
	})
}

func (gi *groupImpl) GetOwner() Actor {
	return gi.owner
}

func (gi *groupImpl) Stop() {
	gi.actors.Range(func(_, value interface{}) bool {
		value.(Actor).Stop()
		return true
	})
}

func (gi *groupImpl) Wait() {
	gi.actors.Range(func(_, value interface{}) bool {
		value.(Actor).Wait()
		return true
	})
}

func (gi *groupImpl) Count() int {
	count := 0
	gi.actors.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

package avatar

import (
	"context"
	"reflect"

	"github.com/echo766/pitaya/component"
	"github.com/echo766/pitaya/logger"
	"github.com/echo766/pitaya/util"
)

const (
	ASF_NULL = iota
	ASF_ONLINE
	ASF_OFFLINE
	ASF_AFK
)

type (
	Avatar struct {
		state int32
		uid   int64
		comps []Component
		evts  map[string][]*component.Handler

		bag Component
	}
)

func NewAvatar() *Avatar {
	a := &Avatar{}
	return a
}

func (a *Avatar) Init(uid int64) bool {
	a.uid = uid
	a.evts = make(map[string][]*component.Handler)

	a.bag = newBagComp()
	a.comps = append(a.comps, a.bag)

	for _, comp := range a.comps {
		comp.Init(a)
	}

	return true
}

func (a *Avatar) RegistEvent(evts map[string][]*component.Handler) {
	for name := range evts {
		a.evts[name] = append(a.evts[name], evts[name]...)
	}
}

func (a *Avatar) IsAFK() bool {
	return a.state == ASF_AFK || a.state == ASF_NULL
}

func (a *Avatar) Comps() []Component {
	return a.comps
}

func (a *Avatar) dispatchEvt(h *component.Handler, data interface{}) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Errorf(
				"avatar dispatch event panic. evt %v.%v err %w \n",
				h.Receiver.Type().Name(),
				h.Method.Type.Name(), err,
			)
		}
	}()

	args := []reflect.Value{h.Receiver, reflect.ValueOf(data)}

	_, err := util.Pcall(h.Method, args)
	if err != nil {
		logger.Log.Errorf("avatar dispatch event error. evt %v.%v err %w \n",
			h.Receiver.Type().Name(),
			h.Method.Type.Name(), err)
	}
}

func (a *Avatar) Emit(evt string, data interface{}) {
	hs, ok := a.evts[evt]
	if !ok {
		return
	}
	for _, h := range hs {
		a.dispatchEvt(h, data)
	}
}

func (a *Avatar) Login(ctx context.Context, msg []byte) {

}

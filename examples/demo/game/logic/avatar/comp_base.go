package avatar

type (
	Component interface {
		Init(*Avatar)
		Name() string
		OnComponentInit()
		Emit(evt string, data interface{})
	}

	baseComp struct {
		owner *Avatar
	}
)

func (c *baseComp) Init(owner *Avatar) {
	c.owner = owner
}

func (c *baseComp) Emit(evt string, data interface{}) {
	c.owner.Emit(evt, data)
}

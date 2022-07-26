package avatar

import "context"

const (
	BAG_COMP_NAME = "bag"
)

type (
	BagComponent struct {
		baseComp
	}
)

func newBagComp() Component {
	comp := &BagComponent{}
	return comp
}

func (c *BagComponent) Name() string {
	return BAG_COMP_NAME
}

func (c *BagComponent) OnComponentInit() {
}

func (c *BagComponent) UseItem(ctx context.Context, data []byte) {
}

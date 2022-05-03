package lifecycle

import (
	"github.com/buildpacks/lifecycle/buildpack"
)

type OrderHandler interface {
	PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order)
}

type DefaultOrderHandler struct{}

func (h *DefaultOrderHandler) PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) {
	if len(orderExt) == 0 {
		return
	}
	for i, group := range orderExt {
		for j := range group.Group {
			group.Group[j].Extension = true
			group.Group[j].Optional = true
		}
		orderExt[i] = group
	}
	extGroupEl := buildpack.GroupElement{OrderExt: orderExt}
	for i, group := range orderBp {
		orderBp[i] = buildpack.Group{
			Group: append([]buildpack.GroupElement{extGroupEl}, group.Group...),
		}
	}
}

type LegacyOrderHandler struct{}

func (h *LegacyOrderHandler) PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) {}

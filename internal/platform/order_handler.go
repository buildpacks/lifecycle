package platform

import (
	"github.com/buildpacks/lifecycle/buildpack"
)

type OrderHandler interface {
	PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) buildpack.Order
}

//go:generate mockgen -package testmock -destination testmock/exec_store.go github.com/buildpacks/lifecycle/internal/platform ExecStore
type ExecStore interface {
	RegisterExtensionsOrder(order buildpack.Order)
}

type DefaultOrderHandler struct {
	execStore ExecStore
}

func NewDefaultOrderHandler(execStore ExecStore) *DefaultOrderHandler {
	return &DefaultOrderHandler{
		execStore: execStore,
	}
}

func (h *DefaultOrderHandler) PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) buildpack.Order {
	h.execStore.RegisterExtensionsOrder(orderExt)
	extGroupEl := buildpack.GroupBuildpack{
		ID:      "cnb_order-ext",
		Version: "",
	}
	for i, group := range orderBp {
		orderBp[i] = buildpack.Group{
			Group: append([]buildpack.GroupBuildpack{extGroupEl}, group.Group...),
		}
	}
	return orderBp
}

type LegacyOrderHandler struct{}

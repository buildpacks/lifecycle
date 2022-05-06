package lifecycle

import (
	"github.com/buildpacks/lifecycle/buildpack"
)

func PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) buildpack.Order {
	if len(orderExt) == 0 {
		return orderBp
	}

	// fill in values for extensions order
	for i, group := range orderExt {
		for j := range group.Group {
			group.Group[j].Extension = true
			group.Group[j].Optional = true
		}
		orderExt[i] = group
	}

	var newOrder buildpack.Order
	extGroupEl := buildpack.GroupElement{OrderExt: orderExt}
	for _, group := range orderBp {
		newOrder = append(newOrder, buildpack.Group{
			Group: append([]buildpack.GroupElement{extGroupEl}, group.Group...),
		})
	}
	return newOrder
}

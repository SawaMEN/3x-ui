package sub

import "github.com/gin-gonic/gin"

func registerAutoSubscriptionRoute(g *gin.RouterGroup, controller *SUBController) {
	if g == nil || controller == nil {
		return
	}
	gLink := g.Group(controller.subPath)
	gLink.GET(":subid/auto", controller.subs)
	gLink.HEAD(":subid/auto", controller.subs)
}

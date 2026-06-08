package controller

import (
	"edu_market/dto/request"
	"edu_market/service"
	"edu_market/utils"

	"github.com/gin-gonic/gin"
)

// OrderController 订单控制器
type OrderController struct {
	svc service.OrderService
}

// Create 创建订单
func (ctr *OrderController) Create(c *gin.Context) {
	var req request.CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	order, err := ctr.svc.Create(userID, req.CourseID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Created(c, order)
}

// List 当前用户订单列表
func (ctr *OrderController) List(c *gin.Context) {
	var req request.OrderListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	userID := c.GetUint("user_id")
	orders, total, err := ctr.svc.ListByUser(userID, req.Page, req.PageSize, req.Status)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	req.Page, req.PageSize = service.GetPagination(req.Page, req.PageSize)
	utils.PageSuccess(c, orders, total, req.Page, req.PageSize)
}

// Pay 模拟支付订单
func (ctr *OrderController) Pay(c *gin.Context) {
	orderNo := c.Param("order_no")
	userID := c.GetUint("user_id")

	if err := ctr.svc.Pay(orderNo, userID); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, nil)
}

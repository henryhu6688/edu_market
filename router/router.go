package router

import (
	"edu_market/controller"
	"edu_market/middleware"
	"edu_market/service"

	"github.com/gin-gonic/gin"
)

// Setup 注册所有路由
func Setup() *gin.Engine {
	r := gin.Default()

	// 全局中间件
	r.Use(middleware.Cors())
	r.Use(middleware.Logger())
	r.Use(middleware.RateLimit())

	// 静态文件（上传文件访问）
	r.Static("/uploads", "./uploads")

	// 初始化控制器
	authCtrl := &controller.AuthController{}
	userCtrl := &controller.UserController{}
	courseCtrl := &controller.CourseController{}
	orderCtrl := &controller.OrderController{}
	reviewCtrl := &controller.ReviewController{}
	categoryCtrl := &controller.CategoryController{}

	// Agent 系统初始化
	agentEngine := service.NewAgentEngine()
	agentSvc := service.NewAgentService(agentEngine)
	agentCtrl := controller.NewAgentController(agentSvc)
	materialCtrl := controller.NewMaterialController()
	documentCtrl := controller.NewDocumentController()

	api := r.Group("/api")
	{
		// 公开接口
		api.GET("/captcha/image", authCtrl.GenerateCaptcha)
		api.POST("/send-code", authCtrl.SendCode)
		api.POST("/login", authCtrl.LoginByCode)
		api.POST("/refresh", authCtrl.Refresh)
		api.GET("/courses", courseCtrl.List)
		api.GET("/courses/:id", courseCtrl.GetByID)
		api.GET("/courses/:id/reviews", reviewCtrl.ListByCourse)
		api.GET("/categories", categoryCtrl.List)
			api.GET("/materials", materialCtrl.List)
			api.GET("/materials/:id", materialCtrl.GetByID)
			api.GET("/materials/:id/documents", documentCtrl.GetTree)

		// 需认证
		auth := api.Group("")
		auth.Use(middleware.JWTAuth())
		{
			// 用户
			auth.GET("/user/profile", userCtrl.GetProfile)
			auth.PUT("/user/profile", userCtrl.UpdateProfile)
			// 订单
			auth.POST("/orders", orderCtrl.Create)
			auth.GET("/orders", orderCtrl.List)
			auth.POST("/orders/:order_no/pay", orderCtrl.Pay)
			// Agent
			auth.POST("/agent/chat", agentCtrl.Chat)
			auth.GET("/agent/sessions", agentCtrl.GetSessions)
			auth.DELETE("/agent/sessions/:id", agentCtrl.DeleteSession)
			auth.GET("/agent/sessions/:id/messages", agentCtrl.GetMessages)
			// 评论
			auth.POST("/reviews", reviewCtrl.Create)
			// 资料
			auth.POST("/materials", materialCtrl.Create)
			auth.PUT("/materials/:id", materialCtrl.Update)
			auth.DELETE("/materials/:id", materialCtrl.Delete)
			auth.GET("/user/materials", materialCtrl.MyMaterials)
			// 文档
			auth.GET("/documents/:id", documentCtrl.GetByID)
			auth.POST("/materials/:id/documents", documentCtrl.Create)
			auth.POST("/materials/:id/documents/upload", documentCtrl.Upload)
			auth.PUT("/documents/:id", documentCtrl.Update)
			auth.DELETE("/documents/:id", documentCtrl.Delete)
		}

		// 管理员
		admin := api.Group("/admin")
		admin.Use(middleware.JWTAuth(), middleware.AdminOnly())
		{
			admin.POST("/courses", courseCtrl.Create)
			admin.PUT("/courses/:id", courseCtrl.Update)
			admin.DELETE("/courses/:id", courseCtrl.Delete)
			admin.POST("/categories", categoryCtrl.Create)
			admin.PUT("/categories/:id", categoryCtrl.Update)
			admin.DELETE("/categories/:id", categoryCtrl.Delete)
		}
	}

	return r
}

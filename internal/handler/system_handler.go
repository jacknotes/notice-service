package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"notice-service/internal/repository"
)

// heartbeatHealthyWindow 心跳健康判定窗口：超过该时长未上报视为离线。
// 与实例心跳上报间隔（5s）保持数倍余量，容忍网络抖动。
const heartbeatHealthyWindow = 15 * time.Second

type SystemHandler struct {
	heartbeats *repository.HeartbeatRepo
	// buildVersion/instanceID 来自启动注入（ldflags / 随机生成），
	// 供「当前服务版本」接口与节点心跳口径保持一致。
	buildVersion string
	instanceID   string
	// swaggerEnabled 是否暴露 /swagger API 文档；前端据此决定「API 文档」入口
	// 是打开文档页还是提示已关闭（避免点击后新标签页被 SPA 兜底跳回首页）。
	swaggerEnabled bool
}

func NewSystemHandler(db *sql.DB, buildVersion, instanceID string, swaggerEnabled bool) *SystemHandler {
	return &SystemHandler{heartbeats: repository.NewHeartbeatRepo(db), buildVersion: buildVersion, instanceID: instanceID, swaggerEnabled: swaggerEnabled}
}

// Version 当前实例构建版本（与节点心跳上报的 version 同源）。
// @Summary 当前服务版本
// @Tags 系统
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/system/version [get]
func (h *SystemHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": h.buildVersion, "instance_id": h.instanceID, "swagger_enabled": h.swaggerEnabled})
}

// Instances 返回全部后端实例节点及健康状态（多实例「信号在线」）。
// @Summary 后端节点健康列表（多实例信号在线）
// @Tags 系统
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/instances [get]
func (h *SystemHandler) Instances(c *gin.Context) {
	list, err := h.heartbeats.List(heartbeatHealthyWindow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	healthy := 0
	for _, it := range list {
		if it.Healthy {
			healthy++
		}
	}
	c.JSON(http.StatusOK, gin.H{"instances": list, "healthy": healthy, "total": len(list)})
}

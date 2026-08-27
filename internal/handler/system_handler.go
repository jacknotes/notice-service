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
}

func NewSystemHandler(db *sql.DB) *SystemHandler {
	return &SystemHandler{heartbeats: repository.NewHeartbeatRepo(db)}
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

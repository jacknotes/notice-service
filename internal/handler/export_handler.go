package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"notice-service/internal/crypto"
	"notice-service/internal/service"
)

type ExportHandler struct {
	svc *service.ExportService
	db  *sql.DB
}

func NewExportHandler(db *sql.DB, cipher *crypto.Cipher) *ExportHandler {
	return &ExportHandler{svc: service.NewExportService(db, cipher), db: db}
}

// Export 导出备份（仅管理员）。
// @Summary 导出渠道/模板/任务 JSON 备份
// @Tags 系统
// @Security BearerAuth
// @Success 200 {object} service.ExportBundle
// @Router /api/export [get]
func (h *ExportHandler) Export(c *gin.Context) {
	bundle, err := h.svc.Export(c.GetInt64("uid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	auditf(c, h.db, "export.data", "导出数据备份（%d 渠道/%d 模板/%d 任务）",
		len(bundle.Channels), len(bundle.Templates), len(bundle.Tasks))
	c.JSON(http.StatusOK, bundle)
}

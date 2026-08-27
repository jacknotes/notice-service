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

func NewExportHandler(db *sql.DB, cipher *crypto.Cipher, sched service.Scheduler) *ExportHandler {
	return &ExportHandler{svc: service.NewExportService(db, cipher, sched), db: db}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "export.data", "导出数据备份（%d 渠道/%d 模板/%d 任务）",
		len(bundle.Channels), len(bundle.Templates), len(bundle.Tasks))
	c.JSON(http.StatusOK, bundle)
}

// Import 导入备份（仅管理员）。
// @Summary 导入渠道/模板/任务 JSON 备份
// @Tags 系统
// @Security BearerAuth
// @Accept json
// @Param body body service.ExportBundle true "备份内容"
// @Success 200 {object} map[string]interface{}
// @Router /api/import [post]
func (h *ExportHandler) Import(c *gin.Context) {
	var bundle service.ExportBundle
	if err := c.ShouldBindJSON(&bundle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	res, err := h.svc.Import(c.GetInt64("uid"), &bundle)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeErr(err)})
		return
	}
	auditf(c, h.db, "import.data", "导入数据备份（+%d 渠道/+%d 模板/+%d 任务，跳过 %d）",
		res.ChannelsCreated, res.TemplatesCreated, res.TasksCreated, len(res.Skipped))
	c.JSON(http.StatusOK, res)
}

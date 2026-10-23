package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"notice-service/internal/crypto"
	"notice-service/internal/model"
)

// TestExportImportTemplateEnabledRoundtrip 回归 H-5：备份导出/导入必须保真模板启用状态。
//
// 缺陷：Import 构造 model.Template 时未带 Enabled，而 template_repo.Create 的 INSERT
// 显式写 enabled 列（`INSERT INTO templates (..., enabled, ...)`），于是 Go 零值 false
// 直接落库、绕过列默认值 1 → 导入后模板一律停用；叠加「模板停用时禁止发送」，
// 迁移到新环境后所有任务静默停发。
//
// 断言：导出 1 启用 + 1 停用模板 → 删除原记录（模拟导入到新环境）→ 导入 → 逐个状态一致。
func TestExportImportTemplateEnabledRoundtrip(t *testing.T) {
	db := testDB(t)
	uid := seedServiceUser(t, db)
	svc := NewTemplateService(db)
	ciph, _ := crypto.New(key32())

	sfx := time.Now().UnixNano()
	onName := fmt.Sprintf("exp_on_%d", sfx)
	offName := fmt.Sprintf("exp_off_%d", sfx)
	mk := func(name string, enabled bool) *model.Template {
		return &model.Template{Name: name, Subject: "s", ContentMD: "c", Enabled: enabled,
			Variables: []model.TemplateVar{}}
	}
	onTpl, offTpl := mk(onName, true), mk(offName, false)
	for _, tpl := range []*model.Template{onTpl, offTpl} {
		if err := svc.Create(uid, tpl); err != nil {
			t.Fatalf("create template %q: %v", tpl.Name, err)
		}
	}
	cleanupNames := func() {
		for _, name := range []string{onName, offName} {
			if list, err := svc.List(uid); err == nil {
				for _, tp := range list {
					if tp.Name == name {
						db.Exec("DELETE FROM templates WHERE id=?", tp.ID)
					}
				}
			}
		}
	}
	t.Cleanup(cleanupNames)

	// 取导出侧的真实载荷：Export 的模板来源就是 TemplateService.List。
	all, err := svc.List(uid)
	if err != nil {
		t.Fatal(err)
	}
	var exported []*model.Template
	for _, tp := range all {
		if tp.Name == onName || tp.Name == offName {
			exported = append(exported, tp)
		}
	}
	if len(exported) != 2 {
		t.Fatalf("exported templates = %d, want 2", len(exported))
	}

	// 备份文件是 JSON：确认 enabled 在序列化/反序列化后仍在（导出侧不丢状态）。
	raw, err := json.Marshal(&ExportBundle{Version: exportVersion, ExportedAt: time.Now(), Templates: exported})
	if err != nil {
		t.Fatal(err)
	}
	var bundle ExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Templates) != 2 {
		t.Fatalf("bundle templates = %d, want 2", len(bundle.Templates))
	}
	// 只导入模板，避免与库内其它渠道/任务（共享数据集）互相干扰。
	bundle.Channels = nil
	bundle.Tasks = nil

	// 模拟「导入到新环境」：先删除原记录（软删），否则会被名称冲突跳过。
	for _, tp := range exported {
		if err := svc.Delete(uid, tp.ID); err != nil {
			t.Fatalf("delete source template %q: %v", tp.Name, err)
		}
	}

	es := NewExportService(db, ciph, &fakeScheduler{})
	res, err := es.Import(uid, &bundle)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.TemplatesCreated != 2 {
		t.Fatalf("templates_created = %d, want 2 (skipped=%v)", res.TemplatesCreated, res.Skipped)
	}

	// 逐个断言状态一致。
	list, err := svc.List(uid)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tp := range list {
		got[tp.Name] = tp.Enabled
	}
	if _, ok := got[onName]; !ok {
		t.Fatalf("imported template %q not found", onName)
	}
	if _, ok := got[offName]; !ok {
		t.Fatalf("imported template %q not found", offName)
	}
	if !got[onName] {
		t.Errorf("imported template %q enabled = false, want true（H-5 回归：Enabled 未被带入）", onName)
	}
	if got[offName] {
		t.Errorf("imported template %q enabled = true, want false（停用状态被导入路径改写）", offName)
	}
}

// TestImportTemplateEnabledWrittenToDB 直接读库断言 enabled 列落库值（绕过 service 层组装），
// 确保修复作用于 template_repo 的显式 INSERT 而非仅内存对象。
func TestImportTemplateEnabledWrittenToDB(t *testing.T) {
	db := testDB(t)
	uid := seedServiceUser(t, db)
	ciph, _ := crypto.New(key32())

	name := fmt.Sprintf("exp_db_%d", time.Now().UnixNano())
	bundle := &ExportBundle{
		Version: exportVersion,
		Templates: []*model.Template{{
			ID: 1, Name: name, Subject: "s", ContentMD: "c", Enabled: true,
			Variables: []model.TemplateVar{},
		}},
	}
	es := NewExportService(db, ciph, &fakeScheduler{})
	res, err := es.Import(uid, bundle)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.TemplatesCreated != 1 {
		t.Fatalf("templates_created = %d, want 1 (skipped=%v)", res.TemplatesCreated, res.Skipped)
	}
	var enabled bool
	if err := db.QueryRow("SELECT enabled FROM templates WHERE name=? AND deleted_at IS NULL", name).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM templates WHERE name=?", name) })
	if !enabled {
		t.Errorf("templates.enabled in DB = 0, want 1（导入丢失启用状态）")
	}
	if !strings.HasPrefix(name, "exp_db_") {
		t.Fatalf("unexpected test template name %q", name)
	}
}

# 发送日志列表列精简 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 精简「发送日志」列表表格：删除 4 个低价值列（触发、触发 IP、错误信息、重试），使表格无横向滚动、时间列完整显示；展开行补「重试次数」，信息零丢失。

**Architecture:** 仅修改 `web/src/views/Logs.vue` 一个文件——删除 4 个 `<el-table-column>` 块、在展开行模板新增「重试次数」detail-block、去掉「操作」列 `fixed="right"`。无后端改动，无需改 `LogDetail.vue`（已展示重试次数）。

**Tech Stack:** Vue 3 `<script setup>`、Element Plus `el-table`。

---

## File Structure

- `web/src/views/Logs.vue` —— 唯一改动文件：
  - 模板中 4 个列定义（触发/触发 IP/错误信息/重试）→ 删除
  - 展开行模板（`.log-detail` 内）→ 新增「重试次数」块
  - 「操作」列 → 去掉 `fixed="right"`

## Task 1: 删除 4 个列 + 去掉操作列 fixed + 展开行补重试次数

**Files:**
- Modify: `web/src/views/Logs.vue:171-211`（触发/触发IP/重试列）、`web/src/views/Logs.vue:206-211`（错误信息列）、`web/src/views/Logs.vue:219`（操作列 fixed）、`web/src/views/Logs.vue:136-139`（展开行错误信息块之后）

- [ ] **Step 1: 删除「触发」列**

删除 `web/src/views/Logs.vue` 第 171-184 行整块：

```vue
        <el-table-column label="触发" min-width="150" sortable="custom" prop="trigger_type">
          <template #default="{ row }">
            <el-tag
              v-if="row.trigger_type"
              :style="triggerTagStyle(row.trigger_type)"
              effect="plain"
              size="small"
            >
              {{ triggerLabel(row.trigger_type) }}
            </el-tag>
            <span v-else class="ok-cell">—</span>
            <span class="mono trigger-by">{{ row.trigger_by || '' }}</span>
          </template>
        </el-table-column>
```

- [ ] **Step 2: 删除「触发 IP」列**

删除 `web/src/views/Logs.vue` 第 186-190 行整块：

```vue
        <el-table-column label="触发 IP" min-width="110" sortable="custom" prop="trigger_ip">
          <template #default="{ row }">
            <span class="mono time-cell">{{ row.trigger_ip || '—' }}</span>
          </template>
        </el-table-column>
```

- [ ] **Step 3: 删除「重试」列**

删除 `web/src/views/Logs.vue` 第 200-204 行整块：

```vue
        <el-table-column label="重试" width="80" align="center" sortable="custom" prop="retry_count">
          <template #default="{ row }">
            <span class="mono retry-cell">{{ row.retry_count ?? 0 }}</span>
          </template>
        </el-table-column>
```

- [ ] **Step 4: 删除「错误信息」列**

删除 `web/src/views/Logs.vue` 第 206-211 行整块：

```vue
        <el-table-column label="错误信息" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.error_msg" class="err-cell">{{ row.error_msg }}</span>
            <span v-else class="ok-cell">—</span>
          </template>
        </el-table-column>
```

- [ ] **Step 5: 「操作」列去掉 fixed**

修改 `web/src/views/Logs.vue` 第 219 行：`<el-table-column label="操作" width="140" align="center" fixed="right">` → 去掉 ` fixed="right"`：

```vue
        <el-table-column label="操作" width="140" align="center">
```

- [ ] **Step 6: 展开行补「重试次数」**

在 `web/src/views/Logs.vue` 展开行模板中，「错误信息」detail-block（第 136-139 行）之后新增：

```vue
              <div v-if="row.retry_count" class="detail-block">
                <span class="detail-label">重试次数</span>
                <span class="mono detail-trigger-text">{{ row.retry_count }}</span>
              </div>
```

> 说明：与 spec 一致——`retry_count` 为 0 / 未定义时该块不渲染（默认发送即成功、无重试时不显示噪音），有重试（>0）时显示次数。此选择与详情页 `{{ log.retry_count ?? 0 }}` 语义不冲突（详情页总是显示，列表展开行仅在有重试时显示）。

- [ ] **Step 7: 清理不再使用的样式**

删除 `web/src/views/Logs.vue` `<style scoped>` 中不再被引用的规则（避免死代码）：
- 第 635-638 行 `.retry-cell { ... }`（重试列删除后无人引用）
- 第 639-643 行 `.trigger-by { ... }`（触发列删除后无人引用——展开行「触发来源」用的是 `.detail-trigger-text`，非本类）
- 第 644-647 行 `.err-cell { ... }`（错误信息列删除后无人引用）

其余样式（`.time-cell`、`.ok-cell`、`.task-name-cell` 等）仍被保留列或展开行使用，保留。

- [ ] **Step 8: 类型检查**

在 `web/` 目录运行类型检查确认无 TS 错误：

```bash
cd web && npx vue-tsc --noEmit -p tsconfig.json
```

Expected: 退出码 0，无报错。

> 注：若项目未配置 vue-tsc 的 tsconfig，可运行 `npx vue-tsc --noEmit`；以实际项目脚本为准（可查 `web/package.json` 的 scripts）。若工具链缺失导致无法运行，至少确认 `web/node_modules/.bin` 下有 `vue-tsc` 或 `tsc`。

- [ ] **Step 9: 构建验证**

```bash
cd web && npm run build
```

Expected: 构建成功，退出码 0，`web/dist` 产物更新。

- [ ] **Step 10: 提交**

```bash
cd /home/jack/trae/notice-service
git add web/src/views/Logs.vue
git commit -m "feat(web): 发送日志列表精简列（删触发/触发IP/错误信息/重试，展开行补重试次数）"
```

---

## Self-Review

- **Spec coverage：**
  - §2.1 删除 4 列 → Task 1 Step 1-4 ✓
  - §2.2 展开行补重试次数 → Task 1 Step 6 ✓
  - §2.3 操作列去掉 fixed → Task 1 Step 5 ✓
  - §2.4 宽度核对（8 列 ~1020px < ~1130px，无需代码改动）→ 已按 2.1-2.5 结果自动达成，无需额外步骤 ✓
  - §3 验收标准 1（8 列）/2（展开行重试次数）/3（无横向滚动）→ 由以上改动达成；标准 4（搜索/筛选/排序/导出不变）→ 无相关代码改动，不受影响；标准 5（build 通过）→ Step 9 ✓
- **Placeholder scan：** 无 TBD/TODO；所有步骤含完整代码/命令 ✓
- **Type consistency：** `row.retry_count` 与 `LogRow` 接口（`retry_count?: number`，Logs.vue:278）一致；模板内 `row` 类型由 `el-table :data="filteredLogs"` 推断，删除的列引用（`triggerLabel`/`triggerTagStyle`/`trigger_by`/`trigger_ip`/`err-cell`/`retry-cell`）中，`triggerLabel`/`triggerTagStyle`/`trigger_by`/`trigger_ip` 仍在展开行「触发来源」块使用，保留；`retry-cell`/`trigger-by`/`err-cell` 样式按 Step 7 清理 ✓

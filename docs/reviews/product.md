# 产品视角审查 · notice-service `v2.0.0..HEAD`

- 审查范围：`git diff v2.0.0..HEAD`（52 个提交，100 个文件，+4751/−396），HEAD = `v2.0.0-52-ga3a914c`
- 视角：产品 / 用户体验，面向自托管运维者与管理员
- 约束：未修改任何业务代码（验证用临时测试文件已删除，工作区仅剩 `.agent-teams/` 未跟踪）
- 方法与证据：读代码 + 实跑可执行验证（见文末「验证记录」）

严重度定义（本视角）：**blocker** = 用户按界面引导操作即得到错误结果且无任何提示；**high** = 主要功能对一类用户静默失效或升级会打断既有部署；**medium** = 明显误导/困扰但有绕过路径；**low** = 文案与一致性瑕疵。

---

## P1 · 农历「中秋」快捷按钮生成的表达式永远不触发，且保存/启动静默成功

- **严重度**：**blocker**（与性能 P1 同源）
- **文件:行号**
  - `web/src/views/Tasks.vue:812`（快捷项 `{ expr: '@lunar term 中秋 09:00', labelKey: 'tasks.lunarMidAutumn' }`）
  - `internal/scheduler/lunar_schedule.go:100-110`（`lunarFestivals` 键为 `中秋节`，非 `中秋`）
  - `internal/service/task_service.go:413-423`（保存校验只判断 `Parse` 是否报错，不判断有无未来触发点）
  - `internal/service/queue.go:345-352`（`RefreshNextRun` 对零值清空 `next_run_at`）
  - `internal/scheduler/scheduler.go` `NextRun`（零值 → `next_run_at` 为空）
- **用户影响**
  1. 管理员在任务表单点「中秋 9点」快捷按钮 → `cron_expr` 被填入 `@lunar term 中秋 09:00`；
  2. 保存返回 **200 成功**，无任何告警；
  3. 任务列表「下次触发时间」列显示 `—`（`Tasks.vue:876-882` 对零值渲染 `—`），任务状态仍是「启用」；
  4. **该任务永远不触发**——中秋当天不会发出任何消息。运维者唯一的线索是 `next_run_at` 为空，而界面把「空」呈现为中性占位符 `—`，与「api 任务/停用任务」的空值视觉完全一致，无法区分「正常无下次」和「配置错了」。
- **独立复核（针对 captain 转来的性能 P1 交叉核实请求）**
  - ① **字符串确实不匹配**：`lunarFestivals` 是精确匹配 map（`lunar_schedule.go:118` `if md, ok := lunarFestivals[name]; ok`），`中秋` ≠ `中秋节`；`term` 的节气回退分支（`lunar_schedule.go:125-133`）用 lunar-go 的 `GetCurrentJieQi().GetName()`，其 24 节气名不含任何「中秋」字样，故两条路径都命中不了。
  - ② **界面没有任何提示或校验**：保存走 `Parse` 成功即通过（`task_service.go:420`），`Parse` 对未知节日名返回 nil error（仅做正则与非空校验，`lunar_parser.go` term 分支只校验 `HH:MM` 与名字非空）；前端 `formRules` 对 `cron_expr` 无自定义校验器。
  - ③ **同类问题确认范围**：
    - 其余 4 个快捷项实测**均正常**（`@lunar monthly 1/15`、`@lunar yearly 1 1`、`@lunar term 清明` 均返回有效下次触发时间）；
    - 但**同一个坑可由用户手工触发**：`@lunar term 元宵 09:00`、`@lunar term 端午 09:00` 同样解析成功、`Next()` 返回零值（实测），而正确写法是 `元宵节` / `端午节`；
    - 更危险的是**列表写法**：`@lunar term 春节,中秋 09:00` 实测 `Next()` **非零**（春节命中）——用户以为「中秋也设上了」，实际中秋永远不会发。这类"部分生效"比全零更隐蔽。
    - `internal/scheduler/lunar_parser.go:17` 与 `:23` 的注释示例写着 `春节,中秋`（短名），**注释本身就是错的**，会引导后续维护者继续使用短名；
    - `web/src/locales/zh-CN.json:422` 的 `lunarHint` 只写 `<节气>`，未说明「农历节日名须用全称（中秋节/元宵节/端午节/除夕）」，用户无从得知正确拼写。
  - ④ **产品严重度判定：blocker**。判据不是「代码 bug 大小」，而是「用户按产品自身提供的引导操作 → 得到静默的错误结果 → 业务后果是漏发通知，且无任何可观测信号」。产品承诺「按你配置的规则，把提醒消息准时发给指定的人」（README:3），此路径直接违背该承诺；且快捷按钮是产品主动提供的引导，属于**产品引导即错误**，不是用户误用。
- **建议**
  1. 后端：`Parse` 阶段对 `term` 的每个名字做**白名单校验**（24 节气 ∪ `lunarFestivals` 键），未知名字返回 `errors.New("未知的节气/节日名「中秋」，可用：中秋节、元宵节、端午节、除夕…（节气请用：清明、立春…）")`——顺带把合法集合直接列给用户，这是最好的输入引导；
  2. 后端：保存时对 `NextRun` 零值给出**分级反馈**——「语法合法但无未来触发点」不阻断（保留合法场景：年份已过），但响应体带 `warning` 字段，前端以黄色提示条展示「该表达式当前无下次触发时间，请确认节日名/年份」；
  3. 前端：快捷项改为 `@lunar term 中秋节 09:00`；`lunarHint` 补全节日全称清单；
  4. 修正 `lunar_parser.go:17,23` 注释中的 `春节,中秋` 示例；
  5. 任务列表：`next_run_at` 为空且 `trigger_type=cron && enabled` 时，不要显示中性的 `—`，改为警示色「无下次触发」并加 tooltip——这是把"静默失败"变成"可见异常"的最小改动。

---

## P2 · 快捷部署文档写错端口：README 说 8080，compose 实际映射 8081

- **严重度**：**high**
- **文件:行号**
  - `README.md:90`（「启动完成后访问 **http://<主机IP>:8080**」）
  - `docker-compose.quickstart.yml:18`（`ports: "8081:8080"`）
  - `docker-compose.quickstart.yml:9`（compose 自身注释写的是 8081，与 README 矛盾）
- **用户影响**：这是「一键部署」路径的第 3 步，面向**只想用起来、不读源码**的用户。按 README 打开 `http://<IP>:8080` 会得到连接失败/超时，而实际服务在 8081。新用户在第一分钟就会卡住，且没有任何错误信息能提示「端口不对」。对照 `docs/deployment.md:21,124-125`（多实例形态确实用 8080/8081）可知 8081 是 quickstart 的真实端口，README 的 8080 是错误陈述。
- **补充证据**：v2.0.0 版本同样存在该不一致（`git show v2.0.0:README.md` 亦写 8080），说明这是长期未被发现的文档缺陷；本次 README 大改（+287 行）时仍未修正，属于「文档与实现脱节」的典型。
- **建议**：`README.md:90` 改为 8081（并在同段注明「compose 中 `ports: "8081:8080"`，如需 8080 请自行改映射」）；或反过来把 quickstart 端口改回 8080 以对齐绝大多数用户预期。二者选一，必须一致。

---

## P3 · `STRICT_SECRETS=1` 默认开启，但 `.env.example` 的占位值不会被判定为弱密钥——「照抄即通过」的虚假安全感

- **严重度**：**high**
- **文件:行号**
  - `.env.example:7-10`（`JWT_SECRET=please_change_me_to_32_bytes_random_hex`、`ENCRYPT_KEY=please_change_me_to_32_bytes_random_hex`、`ADMIN_PASS=please_change_me_to_a_strong_password`）
  - `.env.example:3`（「这些占位值只是让 compose 的 `:?` 守卫通过，直接 cp 后不改 = 弱密钥裸跑；建议同时设置 `STRICT_SECRETS=1`，密钥未改时服务拒绝启动」）
  - `internal/config/config.go:102-117`（`WeakSecretWarnings` 只匹配 `change-me`/`change_me`/`0123456789abcdef0123456789abcdef`/`admin123`/`change-me`，以及 JWT 长度 < 32）
  - `cmd/server/main.go:120-128`（`STRICT_SECRETS=1` 且有告警 → `log.Fatalf` 拒绝启动）
  - `docker-compose.quickstart.yml:29`、`docker-compose.yml:34,63`（`STRICT_SECRETS=${STRICT_SECRETS:-1}`，**默认就是 1**）
- **用户影响**：文档承诺「密钥未改时服务拒绝启动」，实际**不会**拒绝。实测：把 `.env.example` 的三个占位值原样喂给 `WeakSecretWarnings()`，返回 **0 条告警**（占位串长度 ≥32 且不等于任何黑名单值）。于是：
  1. 用户 `cp .env.example .env` 后不改密钥 → 服务**正常启动**，`STRICT_SECRETS=1` 完全失效；
  2. 用户看到启动成功，会合理地认为「密钥已满足要求」；
  3. 实际后果：`JWT_SECRET` 是公开仓库里的固定字符串 → 任何人可伪造任意用户（含 admin）的 JWT；`ADMIN_PASS` 是公开占位符 → 可直接登录默认管理员。
  
  这是**安全文档与安全机制的口径不一致**：安全侧（P2 级的密钥强度校验）已经建好，但产品侧的示例值绕过了它。比「没有严格模式」更糟——因为它给了用户一个"我已按文档做了"的错误确信。
- **建议**
  1. 把 `please_change_me*` 系列加入 `WeakSecretWarnings` 的黑名单（最简单、改动最小）；
  2. 或改为「`STRICT_SECRETS=1` 时要求 `JWT_SECRET`/`ENCRYPT_KEY`/`ADMIN_PASS` 不得包含 `change_me`/`please`/`example`/`your_` 等占位词」，从根上覆盖未来新增的示例串；
  3. `.env.example` 里不要放「看起来合规」的占位值，改为留空 + 注释说明如何生成（compose 的 `:?` 守卫会拦住空值，正好实现文档承诺的行为）；
  4. 补一条回归测试：`WeakSecretWarnings(&Config{JWTSecret: "please_change_me_...", ...})` 必须非空——现有 `config_test.go:104-120` 只测了 `change-me` 与 `admin123`，漏了示例文件里的真实占位串。

---

## P4 · 镜像 tag 策略自相矛盾：文档说 `latest`，compose 钉死一个 `-g<sha>` 版本号

- **严重度**：**high**
- **文件:行号**
  - `docker-compose.quickstart.yml:16`（`image: jacknotes/notice-service:v2.0.0-48-g135ab47`）
  - `README.md:64`（「直接拉取已发布镜像 `jacknotes/notice-service:latest`」）
  - `README.md:101`（「镜像版本固定在 compose 文件中的 `image: jacknotes/notice-service:latest`」——与文件实际内容不符）
  - `README.md:97-98`（`pull && up -d` 升级命令）
- **用户影响**：三个问题叠加：
  1. **文档描述与文件不符**：README:101 说 compose 里固定的是 `latest`，实际是 `v2.0.0-48-g135ab47`。用户按 README 去 compose 里找 `latest` 会找不到。
  2. **`pull` 升级失效**：用户照 README:97-98 执行 `docker compose ... pull && up -d`，因为 tag 是固定版本号，`pull` 拉到的还是同一个镜像，**升级静默无效**——命令成功、容器重启、版本没变。用户只能靠「侧边栏底部版本号」（README:101 提到的功能）自己发现没升上去。
  3. **`-g<sha>` 不是语义化版本**：`v2.0.0-48-g135ab47` 是 `git describe` 的中间产物，指向 48 个提交后的开发快照，不是发布版。把它写进面向最终用户的 quickstart 文件，等于把「开发快照」当作「稳定版」分发；同时 `deploy/publish.sh:1-15` 的文档又说 `latest` 始终指向最新构建——两套口径。
- **建议**：quickstart 统一用 `latest`（与 README:64 的承诺一致），或统一用**语义化发布 tag**（如 `v2.1.0`）；`-g<sha>` 版本号只应出现在 CI/开发环境。README:101 必须按 compose 实际内容重写。

---

## P5 · 文档示例 cron `0 */30 * * * *`（6 段）在本次改动后变成硬错误，但文案与校验信息未同步

- **严重度**：**medium**
- **文件:行号**
  - `web/src/locales/zh-CN.json:369` / `en-US.json:369`（`cronExprPlaceholder`: 「例如：0 */30 * * * *」）
  - `internal/scheduler/lunar_parser.go:51-56`（`looksLikeYearSpec`：6 段一律当作「分 时 日 月 周 年」处理）
  - `internal/scheduler/yearly_parser.go:180-186`（6 段时第 6 段按年份解析）
- **用户影响**：v2.0.0 里 `0 */30 * * * *` 走 robfig 标准解析器，**是合法的**（robfig 支持可选的秒字段）。本次为支持年份字段，把「6 段」无条件解释为「带年份」，于是：
  - 实测 `0 */30 * * * *` → `年份必须是数字: "*"`，保存被拒绝；
  - 而**输入框的 placeholder 示例正是这个表达式**——用户照示例输入必然报错，报错文案「年份必须是数字」对一个只想「每 30 分钟」的用户完全不可理解（他不知道自己在写年份）；
  - 这是**升级引入的行为回归**：升级前保存成功的任务表达式，升级后若重新编辑保存会突然报错（DB 里已有的任务仍能跑，但用户一编辑就卡住）。
- **补充**：`cronHint`（`zh-CN.json:370`）解释的是「5 段 + 末位追加年份」，与 placeholder 的 6 段示例互相矛盾；`README.md` 的 API 概览里也没有 cron 语法说明。
- **建议**
  1. 把 placeholder 改成合法的 5 段示例（如 `0 */30 * * *`）——这是最小修复；
  2. 若有意保留 6 段兼容（秒字段），则 `looksLikeYearSpec` 需要在「第 6 段像年份」时才走年份分支，否则退回标准解析器，并在报错文案中区分「这是年份字段」；
  3. 错误文案面向用户改写：`cron 表达式不合法: 年份必须是数字: "*"` → 「第 6 段被识别为年份字段，但 `*` 不是年份。若要每 30 分钟，请用 5 段：`0 */30 * * *`」；
  4. `cronHint` 与 placeholder 必须口径一致。

---

## P6 · 批量操作失败一律提示「批量删除失败」，与实际操作不符

- **严重度**：**medium**
- **文件:行号**（本次新增的批量操作体系，全部沿用了旧的错误 key）
  - `web/src/views/Tasks.vue:1295`（批量启停失败）、`:1314`（批量改分类）、`:1333`（批量改渠道）、`:1353`（批量改接收地址）
  - `web/src/views/Channels.vue:677`（批量启停）、`:696`（批量改分类）
  - `web/src/views/Templates.vue:658`（批量启停）、`:677`（批量改分类）
  - `web/src/views/Users.vue:848`（批量重置密码）
  - 文案源：`web/src/locales/zh-CN.json:34`（`"batchDeleteFailed": "批量删除失败"`）
- **用户影响**：管理员点「批量启用」失败，看到红色提示「**批量删除失败**」——会误以为自己触发了删除、并担心数据被删。这是「批量操作体系」这个本次重点交付物上最直接的信任损伤点：5 个页面、8 处调用点、全部指向删除文案。
- **注**：该问题在 v2.0.0 已存在于 Channels/Templates 的批量启停（`git show v2.0.0:web/src/views/Channels.vue:646-648`），本次**批量操作体系扩展后扩散到任务页 4 处 + 用户页 1 处**，属"新功能沿用了既有缺陷"，产品上应视为本次交付的一部分。
- **建议**：新增 `common.batchToggleFailed` / `batchCategoryFailed` / `batchChannelFailed` / `batchReceiversFailed` / `batchResetPasswordFailed`，各调用点按语义取用；同时把「批量删除」的确认文案（`common.batchDeleteConfirmMsg`）与结果文案区分开，避免误读。

---

## P7 · 停用模板后，任务侧无任何提示，只能靠发送日志里的失败行发现

- **严重度**：**medium**
- **文件:行号**
  - `internal/service/notification_service.go:67-80`（模板 `!Enabled` → 对每个渠道写一条 `status=failed` 日志、`ErrorMsg="模板「X」已停用"`，然后 `return nil`）
  - `web/src/views/Templates.vue:103-114`（模板列表「状态」开关）
  - `web/src/views/Tasks.vue:245-259`（任务表单的模板下拉，**不过滤停用模板**，无标记）
- **用户影响**：
  1. 管理员在模板页停用一个模板（界面提示「模板已停用」，`zh-CN.json:299`）；
  2. 所有引用该模板的任务仍然显示「启用」、`next_run_at` 照常计算、列表上看不出任何异常；
  3. 到点触发后，`SendTask` 返回 **nil**（不触发重试，这是合理设计），只在发送日志里留下一条 failed 记录；
  4. 运维者要发现"某个任务已经发不出去"，必须主动去翻发送日志。
  
  产品上「停用」语义不完整：模板停用 → 关联任务实际失效，但界面上**任务与模板之间没有任何可见的失效联动**。相比之下渠道停用（`notification_service.go:117-125`）也是同样处理，但渠道停用至少在任务表单里能看到渠道状态。模板这一侧完全断裂。
- **建议**
  1. 任务列表：绑定的模板已停用时，在「通知模板」列（`Tasks.vue:135-147`）加一个「模板已停用」警示标签（数据已在 `templates.value` 里，`templateName()` 旁顺手取 `enabled` 即可，零额外请求）；
  2. 任务表单的模板下拉（`Tasks.vue:253-258`）对停用模板加 `disabled` + 「(已停用)」后缀；
  3. 停用模板时，若存在引用它的启用任务，确认框里明确列出「该模板被 N 个启用任务引用，停用后这些任务将不再发送」——这是本次「模板 enabled」功能最需要补的一句用户告知。

---

## P8 · 农历节日名白名单未覆盖 lunar-go 全部节日，用户手工输入的正确短名会静默失效

- **严重度**：**medium**
- **文件:行号**
  - `internal/scheduler/lunar_schedule.go:100-110`（`lunarFestivals` 仅 9 项：春节/元宵节/龙头节/端午节/七夕/中秋节/重阳节/腊八节/除夕）
  - `internal/scheduler/lunar_parser.go:23`（注释示例使用短名 `春节,中秋`，与实际白名单不符）
  - `web/src/locales/zh-CN.json:422`（`lunarHint` 只说 `<节气>`，未列合法节日名）
- **用户影响**：lunar-go 的 `LunarUtil.FESTIVAL` 覆盖的节日多于本项目的 9 项，而本项目的 `term` 分支在节日表未命中时回退到节气匹配——节气名集合与节日名集合**没有交集**，于是任何「既不在 9 项节日表、也不是 24 节气」的名字都表现为：解析成功、`Next()` 零值、任务永不触发（与 P1 同一失败模式，只是入口从快捷按钮变成手工输入）。实测 `@lunar term 元宵 09:00`、`@lunar term 端午 09:00` 均为零值。
- **建议**：与 P1 合并修复（白名单校验 + 合法值提示）。产品上必须做到：**输入非法节日名时保存即报错并列出可用名字**，而不是静默存下一个永不触发的任务。

---

## P9 · 会话窗口逻辑改变了「登录态」语义，但只在 README 深处一句话说明，无升级告知

- **严重度**：**medium**
- **文件:行号**
  - `web/src/utils/session.ts:1-104`（会话 Cookie + sessionStorage 窗口标记三重判定）
  - `web/src/stores/auth.ts:1-40`（`initSession()` 在 store 模块加载期执行）
  - `README.md:380`（唯一说明：「浏览器不关闭时保持登录，关闭浏览器后需重新登录」）
  - `CHANGELOG.md:7`（`## [Unreleased]` 为空——本次 52 个提交的会话相关改动（`9d172be`/`380ebfa`/`5b9be09`）均未记录）
- **用户影响**：
  1. **行为变更无告知**：升级前 token 在 `localStorage` 里长期有效（关浏览器重开仍是登录态）；升级后关浏览器即需重新登录。对自托管运维者而言这是**日常体验的显著变化**，而 README 只在「形态四：仅前端部署」这一节里顺带提了一句，CHANGELOG 的 `Unreleased` 段完全空白。
  2. **多标签页/复制标签页的边界行为**：`session.ts:60-100` 用「Cookie 缺失 + 有窗口标记 → 视为复制标签页竞态，保持登录」这一启发式。产品上这意味着「浏览器关闭重开」与「复制标签页」的判定依赖浏览器对 sessionStorage 的复制行为，属**实现细节泄漏为用户可见行为**——用户无法预测自己这次开标签页是否还登录着。
  3. 一旦 `initSession` 误判为「浏览器重开」→ `clearSession()` 清掉 token，用户被踢到登录页且**没有任何提示说明原因**（不是 401，是被前端主动清除）。
- **建议**
  1. 在 CHANGELOG 的 `Unreleased`/新版本段明确记录「登录态改为窗口级（关闭浏览器需重新登录）」，并在 README 的「升级」小节给出提示；
  2. 若产品意图是「保持长期登录」，则应提供配置项（如 `SESSION_SCOPE=window|persistent`），让运维者选择——目前是硬编码行为；
  3. 被 `clearSession()` 主动登出时，登录页应显示一句「登录状态已过期，请重新登录」，而不是静默跳转。

---

## P10 · 「下次触发时间」为空时无法区分「未配置」与「配置错误」

- **严重度**：**medium**
- **文件:行号**
  - `web/src/views/Tasks.vue:123-127`（`next_run_at` 列）
  - `web/src/views/Tasks.vue:876-882`（`fmtTime`：空值/零值统一返回 `—`）
  - `web/src/views/Tasks.vue:680-697`（「已过期」筛选依赖 `next_run_at` 存在，空值的任务在「已过期」筛选下**不命中**——见 `:683` `if (!t.next_run_at) return false`）
- **用户影响**：`next_run_at` 为空有三种完全不同的成因：① api 触发任务（正常）；② 任务已停用（正常）；③ cron 表达式无未来触发点（**故障**，见 P1/P8）。界面用同一个 `—` 表示，且「已过期」筛选会**主动隐藏**第 ③ 类任务（因为 `next_run_at` 为空 → 不命中"已过期"）。于是用户想通过「已过期」筛选排查异常任务时，恰好把最需要排查的那批任务过滤掉了。
- **建议**
  1. 区分渲染：`trigger_type=cron && enabled && !next_run_at` → 显示警示「无下次触发」+ tooltip「表达式无未来触发点，请检查节日名/年份」；
  2. 「已过期」筛选把「cron 启用但 next_run_at 为空」也纳入（它们本质上是"不会再有下次"），或新增独立的「异常」快捷项；
  3. 列表顶部对这类任务给一个汇总提示条（「N 个任务当前不会触发」），把静默故障变成可见信号。

---

## P11 · 保存 cron 表达式「合法但无未来触发点」时，界面显示成功但列表立刻是空的

- **严重度**：**medium**
- **文件:行号**
  - `internal/service/task_service.go:413-423`（注释明确写「解析成功但无未来触发点 → 允许保存」，仅注释，无用户反馈）
  - `internal/service/task_service.go:186-192`（`Update` 后 `RefreshNextRun` → 零值则清空）
  - `web/src/views/Tasks.vue:1150-1176`（`saveTask` 成功只弹「任务已创建/已更新」）
- **用户影响**：设计上刻意允许「年份已过」等场景保存（合理），但产品上完全没有把「这个任务不会触发」告诉用户。典型误用：用户想设「2026-09-15 提醒」，在 2026-10 才录入 → 保存成功、任务列表显示启用、`next_run_at` 为 `—` → 用户以为设好了，实际永远不会发。这与 P1 是同一失败模式的不同入口。
- **建议**：保存响应里带 `next_run_at`（已在 `task_service.go:186-192` 同步进响应体），前端在 `next_run_at` 为空且 `trigger_type=cron && enabled` 时，把成功提示改为**警告**：「任务已保存，但该表达式当前没有下次触发时间（可能年份已过），请确认」。这是一个纯前端、零后端改动的修复。

---

## P12 · 备份导入丢失模板 `enabled` 状态，所有导入的模板一律变成「停用」

- **严重度**：**high**
- **文件:行号**
  - `internal/service/export_service.go:124`（`nt := &model.Template{Name: t.Name, Subject: t.Subject, ContentMD: t.ContentMD, Variables: t.Variables}`——**未带 `Enabled`**，Go 零值 = `false`）
  - 对照：同函数 `:106` 渠道导入带 `Enabled: c.Enabled`、`:142` 任务导入带 `Enabled: t.Enabled`
  - `internal/repository/template_repo.go:17-18`（`INSERT INTO templates (..., enabled, ...) VALUES (..., ?, ...)` 显式写入 `t.Enabled`，因此零值 false 会**直接落库**，不经过列默认值 `1`）
  - `internal/database/migrations/016_templates_enabled.sql`（`NOT NULL DEFAULT 1`——默认值在此路径下不生效）
- **用户影响**：备份/迁移导入后，**所有模板都被写成 `enabled=0`（停用）**，无论导出前是启用还是停用。这不是「状态漂移」，是**状态归零**：
  1. 用户在新环境导入备份 → 渠道正常、任务正常、模板全部停用；
  2. 结合 P7（模板停用时 `notification_service.go:67-80` 只写一条 failed 日志并 `return nil`），**所有任务从此不再发送任何消息**；
  3. 任务列表仍显示「启用」、`next_run_at` 照常计算，界面上完全看不出问题；
  4. 用户会以为"迁移成功了"（导入结果摘要显示 `templates_created: N`，无任何告警）。
  
  这是本次「模板 enabled」交付物在**最需要它正确的地方（数据迁移）失效**，且失败模式与 P7 叠加后完全静默。相比 P7（用户主动停用模板，至少知道自己做了什么），本条的受害者是"什么都没做错、只是迁了个环境"的运维者。
- **建议**
  1. `export_service.go:124` 补 `Enabled: t.Enabled`（一行修复）；
  2. 补回归测试：导出含「1 启用 + 1 停用」模板 → 导入后逐个断言状态一致，与 `export_test.go` 现有用例并列；
  3. 更稳妥：`TemplateService.Create` 的入参若来自导入路径，显式要求带 `enabled`（或用指针 `*bool` 区分"未提供"与"显式 false"），避免任何未来新增的导入入口再踩同一坑。

---

## P13 · 模板 `enabled` 的开关文案在列表与表单不一致（「开/关」vs「启用/停用」）

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Templates.vue:110-111`（列表开关用 `t('tasks.on')`/`t('tasks.off')` → 「开」/「关」）
  - `web/src/views/Templates.vue:184-188`（表单开关用 `t('common.enabled')`/`t('common.disabled')` → 「启用」/「停用」）
  - `web/src/views/Tasks.vue:359`（任务表单用 `common.enabled`/`common.disabled`）
  - `web/src/locales/zh-CN.json:349-350`（`tasks.on: "开"`, `tasks.off: "关"`）
- **用户影响**：同一功能（模板启用状态）在列表显示「开/关」，在编辑弹窗显示「启用/停用」，术语不统一。README:35 与 CHANGELOG:18 都强调过「用户管理『停用』统一为『禁用』」这类术语治理，此处是新引入的不一致。另：`tasks.on/off` 被模板页借用，跨模块耦合。
- **建议**：新增 `common.switchOn/switchOff` 或统一用 `common.enabled/disabled`；避免模板页引用 `tasks.*` 命名空间。

---

## P14 · 英文文案语义偏差：「Next 7 days」实为「未来 7 天」但被当作「最近 7 天」

- **严重度**：**low**
- **文件:行号**
  - `web/src/locales/zh-CN.json:425-426`（`range7Days: "最近7天"`、`range1Month: "最近1个月"`）
  - `web/src/locales/en-US.json:425-426`（`range7Days: "Next 7 days"`、`range1Month: "Next month"`）
  - `web/src/views/Tasks.vue:659-666`（`presetRange(days)` 计算的是**今天起往后** N 天）
- **用户影响**：中文「最近 7 天」通常理解为**过去** 7 天，而筛选逻辑（`nextRunInRange`, `Tasks.vue:681-697`）筛的是**未来**的 `next_run_at`。英文 "Next 7 days" 反而更准确，两个语言对同一功能给出相反的时间方向暗示。用户按中文理解去筛「最近 7 天触发过的任务」，得到的是「未来 7 天将触发的任务」——筛选结果看起来是空的（因为大多任务下次触发在更远的未来），用户会以为筛选坏了。
- **建议**：中文改为「未来 7 天」/「未来 1 个月」，与 `nextRun`（下次触发时间）列的语义对齐；英文保持 "Next 7 days"。

---

## P15 · README 存在 Markdown 转义损坏，`make dev` 段落已不可读

- **严重度**：**low**
- **文件:行号**
  - `README.md:90`（`**http\://<主机IP>:8080**`——反斜杠转义泄漏到正文）
  - `README.md:153-154`（`访问 \*\*<http://localhost:5173**。\`make> dev\`会先执行\`db-start\`（…` 前提仅剩 :8080 未被占用（有旧进程先 \`  kill <pid>\`）。`）
- **用户影响**：README 是自托管用户的第一入口，本次 +287 行大改引入了多处转义损坏。第 153-154 行已经**语法错乱到无法照做**：`make> dev`、`` `  kill <pid>\` `` 都不是可复制的命令，用户会困惑「到底该执行什么」。这属于「部署文档是否可照做成功」的直接否定项。
- **建议**：修正这两处转义（`**http://<主机IP>:8080**`、`**http://localhost:5173**。\`make dev\` 会先执行 \`db-start\`…`）；建议加一个 CI 检查（markdownlint 或简单的「代码块内不得出现 `\*\*`/`\`` 转义」正则）防回归。

---

## P16 · 环境变量表未收录本次新增的 `STRICT_SECRETS`

- **严重度**：**low**
- **文件:行号**
  - `README.md:391-417`（环境变量表，共 20 项，**无 `STRICT_SECRETS`**）
  - `.env.example:28-30`（新增的 `STRICT_SECRETS=1`）
  - `docker-compose.quickstart.yml:29`、`docker-compose.yml:34,63`（compose 默认注入 1）
- **用户影响**：`STRICT_SECRETS` 是本次新增的**行为开关**（会导致服务拒绝启动），默认值为 1，却不在 README 的环境变量表中。运维者遇到「服务启动失败」时，按 README 的表排查会找不到这个变量；也无人知道可以设 `STRICT_SECRETS=0` 降级。同时 `docs/deployment.md` 的密钥表格（`:62-70`）也未提及。
- **建议**：README 环境变量表补 `STRICT_SECRETS | 1 | 检测到弱密钥时拒绝启动；开发环境可设 0 仅告警`；`docs/deployment.md` 的 `.env` 表格同步补一行。

---

## P17 · `docs/deployment.md` 与新默认值脱节（MySQL 5.7 / 无 STRICT_SECRETS）

- **严重度**：**low**
- **文件:行号**
  - `docs/deployment.md:97`（`MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:5.7  # MySQL 镜像`）
  - `docs/deployment.md:62-70`（`.env` 关键变量表，无 `STRICT_SECRETS`）
  - 对照 `docker-compose.yml:75`（`image: ${MYSQL_IMAGE:-mysql:8.0}`，本次已升级到 8.0）
- **用户影响**：服务器部署指南仍教用户把 MySQL 钉在 5.7（已 EOL），而仓库默认已改为 8.0。按该文档部署的用户会得到一个与项目默认配置不一致、且已停止安全维护的数据库。文档与实现脱节。
- **建议**：`docs/deployment.md:97` 改为 `mysql:8.0`，并保留「老 Docker 引擎需退回 5.7」的注记（README:105-107 已有该说明，可直接引用）。

---

## P18 · CHANGELOG 的 `[Unreleased]` 为空，本次 52 个提交的重大变更全部未记录

- **严重度**：**medium**
- **文件:行号**
  - `CHANGELOG.md:7`（`## [Unreleased]` 下为空）
  - `CHANGELOG.md:9`（`## [2.0.0] - 2026-09-01` 已归档）
  - 范围：`git rev-list --count v2.0.0..HEAD` = **52 个提交**
- **用户影响**：本次交付的多项**用户可见且行为变更**的功能完全没有 CHANGELOG 条目：
  1. 农历 cron 的 `last`/列表/区间/年份字段（`5d38804`/`6463ba9`/`d2030b6`）——扩展了语法，用户需要知道；
  2. PushPlus ClawBot 子渠道（`135ab47`）——新增渠道类型；
  3. 五渠道排版统一（`d784199`/`42dd791`/`8fee4e5`）——**已发消息的渲染效果会变**，属于用户可感知的输出变更；
  4. 会话窗口逻辑（`9d172be`/`380ebfa`/`5b9be09`）——登录态语义变更（见 P9）；
  5. `STRICT_SECRETS` 弱密钥拒绝启动（`b11aa30`）——**升级后可能直接启动失败**；
  6. MySQL 默认 8.0（`7465812`）——部署形态变更；
  7. 镜像 tag 策略与 `latest` 维护（`12c6f06`/`1982f8f`）——升级流程变更。
  
  自托管用户判断「要不要升级、升级要注意什么」完全依赖 CHANGELOG。空白意味着他们只能盲升，而其中至少 `STRICT_SECRETS` 与「会话窗口」两项会导致升级后**立刻遇到行为变化或启动失败**。
- **建议**：补写 `[Unreleased]`（或直接开 `[2.1.0]`），至少覆盖上述 7 项；把「升级注意」单列一节（`STRICT_SECRETS=1` 默认开启、MySQL 8.0、登录态窗口化）。CHANGELOG 的准确性本身就是本次审查的关注点之一。

---

## P19 · 渠道类型下拉把 PushPlus 拆成「分组子类型」，但任务页的类型名映射未同步

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Channels.vue:287-298`（`typeOptions` 移除 `wechat`，改为 `pushplusSubtypes` 分组：`wechat`/`clawbot`）
  - `web/src/views/Tasks.vue:817-823`（`TYPE_KEY` 映射**只有 `wechat`，无 `clawbot`**）
  - `web/src/views/Tasks.vue:838-845`（`channelTypeLabel`：`TYPE_KEY[c.type]` 未命中则**直接回退显示原始 type 字符串**）
- **用户影响**：任务表单选中 ClawBot 渠道时，「当前渠道为 …」提示（`Tasks.vue:309-311`）会显示 `clawbot` 这个**内部标识符**，而不是本地化名称「新消息ClawBot(5G短信)」。中文界面里突然出现一个英文小写单词，用户不知道那是什么渠道。同类回退在 `Channels.vue:405-408` 也有（但那里 `typeMeta` 已补 `clawbot`，故只有任务页漏了）。
- **建议**：`Tasks.vue:817-823` 补 `clawbot: 'channels.type.clawbot'`；更稳妥的做法是把 `TYPE_KEY`/`typeMeta` 抽成共享常量，避免两处重复维护（本次已经出现一次不同步）。

---

## P20 · ClawBot 渠道的配置项与 PushPlus 完全相同，界面未说明「仅中国移动号」之外的关键差异

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Channels.vue:331-334`（`clawbot` 的 `configFields` 与 `wechat` 完全一致：`pushplus_token` + `pushplus_topic`）
  - `web/src/locales/zh-CN.json:236`（`phPpClawToken`: 「PushPlus token，需先在官网绑定新消息ClawBot（仅中国移动号）」）
  - `internal/channel/clawbot.go:12-24`（短信渠道，正文降级为纯文本）
  - `README.md:15-37`（功能特性列表**未提及 ClawBot / 5G 短信**）
- **用户影响**：
  1. 用户从下拉里选「新消息ClawBot(5G短信)」，看到的表单与「微信服务号」一模一样，仅 placeholder 提示要先在官网绑定、仅移动号——**没有说明"短信是纯文本，Markdown 排版会丢失"**，而这是两个子渠道最大的体验差异（`clawbot.go:19-24` 专门为此做了 `ToPlainText` 降级）；
  2. README 的功能特性与渠道清单完全没提这个新渠道（`README.md:15`「支持邮箱、企业微信、钉钉、飞书、个人微信（PushPlus）」），用户不知道有 5G 短信能力。
- **建议**：ClawBot 的配置表单加一行说明「短信为纯文本，Markdown 格式（标题/列表/链接样式）不会保留」；README 的渠道清单与功能特性补 ClawBot。

---

## P21 · 批量变更分类/渠道/接收地址的确认弹窗是「点确认后才填值」，缺少二次确认

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Tasks.vue:1244-1272`（`onBatchCommand`：分类/渠道/接收地址三项都是**先打开弹窗**，弹窗里再选值）
  - `web/src/views/Tasks.vue:469`、`:490`、`:519`（弹窗正文文案 `batchCategoryConfirmMsg` 等，形如「将选中的 N 项分类变更为「{name}」？」——但此时 `name` 尚未选择，渲染为空字符串）
- **用户影响**：弹窗文案是「确认式」措辞（「将选中的 N 项分类变更为「」？」），而实际交互是「在此选择新值并提交」。空引号 `「」` 在用户填值前就显示出来，读起来像是漏了内容。同时**真正的写操作（点「确认」）没有任何二次确认**——与同页的批量删除（`batchDelete` 有 `ElMessageBox.confirm`）、批量启停（`doBatchToggle` 有 confirm）不一致：破坏性更强的「批量改投递渠道/接收地址」反而一步提交。
- **建议**：把弹窗文案改为指令式（「为选中的 N 项选择新的分类」），并在提交时对「批量改渠道/接收地址」追加一次确认（这两项会覆盖原有配置，且无撤销）。

---

## P22 · 前端测试在本机无法运行（jsdom 30 与 Node 20 不兼容），CI 使用同一 Node 20

- **严重度**：**low**（工程一致性，非用户直接可见；但影响「可执行验证」的可信度）
- **文件:行号**
  - `web/package.json:10`（`"test": "vitest run --passWithNoTests"`）
  - `web/package-lock.json`（`jsdom` 固定 `30.0.1`，其 `engines.node` 为 `^22.22.2 || ^24.15.0 || >=26.0.0`）
  - `.github/workflows/ci.yml:112`（`node-version: '20'`）、`:121-123`（`npm run test`）
- **实测**：`cd web && npm test` → 8 个 unhandled error（`TypeError: webidl.util.markAsUncloneable is not a function`），`Test Files no tests / Tests no tests`，退出码 1。本机 Node v20.20.2。`npx vue-tsc --noEmit` 通过（退出码 0），说明类型检查正常、测试环境不可用。
- **影响**：`--passWithNoTests` 会掩盖「一个测试都没跑起来」这一事实（本次实测正是「有 8 个错误但 0 个测试」）。CI 用 Node 20 + `npm ci`，若同样触发，则前端测试长期处于**假绿/假红**状态；本地开发者也无法用测试验证改动。
- **建议**：把 CI 的 `node-version` 升到 22/24（与 jsdom 30 的 engines 对齐），或把 `jsdom` 降级到支持 Node 20 的版本；同时去掉 `--passWithNoTests`（或至少改为在 0 测试时报错），避免"没跑测试也算通过"。

---

## P23 · 「已过期」筛选的判定用客户端 `Date.now()` 与字符串解析，跨时区/格式变化时可能误判

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Tasks.vue:681-697`（`new Date(t.next_run_at).getTime()` 与 `new Date(\`${s}T00:00:00\`).getTime()` 混用）
- **用户影响**：`next_run_at` 来自后端 `time.Time` 的 JSON（带时区偏移），`new Date(...)` 会正确解析；但日期区间的 `s`/`e` 是 `value-format="YYYY-MM-DD"` 的本地日期串，拼成 `T00:00:00` 后按**浏览器本地时区**解析。若浏览器时区与服务端 `TZ=Asia/Shanghai` 不同（远程运维常见），「今天」的边界会偏移 8 小时，筛选结果与用户预期不符。产品上属于「筛选结果看起来不对但无提示」。
- **建议**：用后端返回的时区或明确在 UI 上标注筛选按浏览器本地时间；更稳妥的是把「按下次触发时间筛选」下推给后端（与「已过期」同一查询），避免客户端时区推断。

---

## P24 · 任务表格新增「下次触发时间」「通知模板」两列后，窄屏下横向滚动加剧

- **严重度**：**low**
- **文件:行号**
  - `web/src/views/Tasks.vue:123`（新增 `nextRun` 列 `min-width=150`）
  - `web/src/views/Tasks.vue:135`（新增 `template` 列 `min-width=140`）
  - `web/src/views/Tasks.vue:95-99`（分类列从 `width=130` 改 `width=120`，仅省 10px）
  - `web/src/views/Tasks.vue:1697`（唯一一处 `@media (max-width: 480px)`）
- **用户影响**：本次给任务表加了 2 列（+290px 最小宽度），只省回 10px。README:35 声称「PC/移动端响应式」，但任务页在移动端/窄窗口下需要横向滚动才能看到「状态」「操作」列（操作列已 `fixed="right"` 缓解，但中间列仍拥挤）。移动端运维（手机上看通知是否正常）是自托管场景的真实需求。
- **建议**：窄屏下把「分类/模板/接收地址」折叠进展开行或 `show-overflow-tooltip` 的合并列；或在 `@media (max-width: 768px)` 下默认隐藏次要列（已有 `index.css:128` 的断点可复用）。

---

# 最高优先级结论（产品视角 Top 5）

1. **P1（blocker）· 农历「中秋」快捷按钮产出永不触发的任务，且保存静默成功。** 产品主动提供的快捷引导直接指向一个错误表达式（`Tasks.vue:812` 的 `中秋` vs `lunar_schedule.go:106` 的 `中秋节`），保存校验只看语法不看"有无下次触发"，界面把空 `next_run_at` 渲染成中性的 `—`。业务后果是**漏发通知且无任何可观测信号**。修复要点不在改一个字符串，而在补齐三道产品护栏：**节日名白名单校验（保存即报错并列出可用名字）→ 零值触发点警告 → 列表把"无下次触发"显示为异常**。已独立复现，与性能 P1 同源，建议合成报告合并为一条并保留双方证据。

2. **P2 + P4（high）· 「一键部署」这条最主力的上手路径有两处硬伤：端口写错（README 8080 / compose 8081）、镜像 tag 策略自相矛盾（文档说 `latest`，文件钉 `-g<sha>`）。** 前者让新用户在第一分钟连不上；后者让 `pull && up -d` 升级静默无效。自托管产品的口碑主要取决于首次部署成功率，这两条必须与 README 大改一起修掉。

3. **P3（high）· `STRICT_SECRETS=1` 的承诺是假的：`.env.example` 的占位密钥能通过弱密钥检测（实测 0 条告警），服务照常启动。** 这比没有严格模式更危险——用户会因为"启动成功"而确信自己已按安全要求配置，实际 JWT 可伪造、默认管理员可直接登录。修复很小（占位串进黑名单 + 回归测试），但产品含义是「文档承诺必须由代码兑现」。

4. **P7 + P10 + P11 + P12（high/medium，同一主题）· 本次交付的几个"状态开关"缺少失效可见性：备份导入把模板一律写成停用（P12，high）、模板停用后关联任务毫无提示（P7）、`next_run_at` 空值无法区分正常与故障（P10）、"合法但永不触发"保存后只报成功（P11）。** 共同模式是「配置生效了，但用户看不出它导致了什么」。P12 尤其致命：它让**什么都没做错、只是迁移了环境**的运维者在无任何告警的情况下失去全部通知能力。产品上应统一为一条原则：**任何让任务停止投递的状态变更，都必须在该任务所在列表上留下可见标记**；导入路径必须逐字段还原状态。

5. **P18（medium）· CHANGELOG 的 `[Unreleased]` 为空，52 个提交的重大变更零记录，其中 `STRICT_SECRETS`（升级后可能启动失败）、MySQL 8.0、登录态窗口化（关浏览器即登出）三项会直接改变升级体验。** 自托管用户判断"要不要升、升了要注意什么"完全依赖 CHANGELOG；空白等于让他们盲升。这同时是审查关注点「CHANGELOG 是否准确反映 52 个提交」的直接否定结论。

---

# 验证记录（可复现）

所有验证均在 `F:\project\notice-service` 下执行；临时测试文件用后即删，未改动任何业务代码（`git status` 仅 `.agent-teams/` 与本次交付的 `docs/reviews/` 未跟踪）。

| 验证项 | 命令 | 结果 |
|---|---|---|
| 前端类型检查 | `cd web && npx vue-tsc --noEmit` | **通过**（exit 0） |
| 前端单元测试 | `cd web && npm test` | **失败**（8 个 unhandled error，`no tests`，exit 1；jsdom 30 需 Node ≥22，本机 Node v20.20.2）→ 见 P22 |
| 农历/年份调度单测 | `go test ./internal/scheduler/ -run "Lunar\|Year\|Yearly" -count=1` | **通过**（0.327s） |
| 全部调度单测 | `go test ./internal/scheduler/ -count=1` | 失败（`TestLeaseExclusiveAndExpiry`/`TestSchedulerLeasePath` 连接本地 MySQL 超时，环境缺测试库，与本次改动无关） |
| 弱密钥检测 | 临时测试喂 `.env.example` 三个占位值给 `WeakSecretWarnings()` | **返回 0 条告警** → 证实 P3 |
| 农历快捷项逐个解析 | 临时测试对 5 个 UI 快捷项执行 `Parse` + `Next(2026-10-22)` | `monthly 1/15`、`yearly 1 1`、`term 清明` **有效**；`term 中秋` **零值**；`term 中秋节` 有效 → 证实 P1 |
| 节日短名/列表 | 同上，追加 `term 元宵`/`term 端午`/`term 春节,中秋` | 前两者 **零值**；`春节,中秋` **非零**（部分生效，中秋静默丢失）→ 证实 P8 |
| 6 段 cron 行为 | 临时测试 `0 */30 * * * *` 等 7 个表达式 | `0 */30 * * * *` → `年份必须是数字: "*"`（保存被拒）；`0 9 15 9 * 2026` 与 `0 9 15 9 2026` → 零值 → 证实 P5/P11 |
| 端口一致性 | 读 `docker-compose.quickstart.yml:18` vs `README.md:90` | 8081 vs 8080 → 证实 P2 |
| 镜像 tag | 读 `docker-compose.quickstart.yml:16` vs `README.md:64,101` | `v2.0.0-48-g135ab47` vs `latest` → 证实 P4 |
| 备份导入 `Enabled` | 读 `export_service.go:106/124/142` 三处对照 | 渠道✓、任务✓、**模板✗** → 证实 P12 |
| 提交数 | `git rev-list --count v2.0.0..HEAD` | **52**（与任务描述一致） |

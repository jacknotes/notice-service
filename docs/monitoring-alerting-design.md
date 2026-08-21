# 监控告警接入设计（Prometheus · Grafana · Zabbix · Alertmanager）

> 状态：**设计草案（未实现）** · 本文仅描述如何将 notice-service 接入一套
> 「监控 → 告警 → 通知」闭环，供后续实施参考，不在本文范围内落地代码。

## 1. 背景与目标

notice-service 已具备完整的**告警触达**能力：多渠道（邮件/企微/钉钉/飞书/PushPlus）、
模板+变量渲染、异步持久化发送队列（重试/去重/崩溃接管）、发送日志（含触发人/IP/方式）、
操作审计。这套能力恰好是监控体系的「最后一公里」——把告警送到人。

目标：以 **Prometheus** 采集指标、**Grafana** 可视化、**Zabbix** 做主机级存活/资源监控、
**Alertmanager** 统一收口告警，并把**告警转交给 notice-service 的 Webhook API** 完成最终触达。
分层如下：

```
┌──────────────┐    scrape     ┌────────────────┐
│  Prometheus   │◄─────────────│  notice-service │  /metrics 指标
└──────┬───────┘               │  (target)       │
       │  alerting rules       └────────────────┘
       ▼                             ▲
┌──────────────┐    HTTP     ┌──────┴───────┐
│  Alertmanager │◄───────────│   Zabbix     │  (主机存活/资源告警)
└──────┬───────┘             └──────────────┘
       │  route / receiver (webhook)
       ▼
┌──────────────────────────────────────────────┐
│ notice-service  POST /api/webhook/<api_key>  │  ← 复用现有发送管线
│   → 模板渲染 → 邮件/企微/钉钉/飞书/PushPlus    │
└──────────────────────────────────────────────┘
       可视化：Grafana（消费 Prometheus）
```

## 2. 组件职责分工

| 组件 | 职责 | 与 notice-service 的关系 |
|------|------|--------------------------|
| Prometheus | 拉取式采集业务指标、执行告警规则 | 新增 `/metrics` 端点作为 target |
| Grafana | 指标可视化仪表盘 + 内置告警 | 数据源指向 Prometheus；可选替代内置仪表盘 |
| Zabbix | 主机/基础设施级监控（CPU、内存、磁盘、进程、网络、日志） | 独立于业务层，告警可转发进 Alertmanager |
| Alertmanager | 告警收口：去重、分组、静默、抑制、路由 | 把告警按路由发到 notice-service webhook |
| notice-service | 告警的最终触达（通知发送） | 作为 Alertmanager 的 Webhook receiver |

**为什么分工这样划**：Prometheus 关注「业务健康」（这条消息发出去没有、队列积压多少），
Zabbix 关注「机器健康」（服务是不是挂了、磁盘是不是满了），Alertmanager 负责把两者
产生的告警统一去重分组后按优先级路由出去，避免告警风暴。

## 3. notice-service 侧需要的能力

### 3.1 新增 `/metrics` 端点（Prometheus 采集）

当前没有指标端点，这是接入的前提。建议用 `prometheus/client_golang` 暴露一个
`GET /metrics`（认证保护，见 §7 安全），指标命名遵循 Prometheus 约定，建议包括：

**业务指标（核心）**
- `notice_sends_total{channel,status}`：发送量计数器（success/failed，按渠道）
- `notice_send_duration_seconds{channel}`：单次发送耗时直方图
- `notice_queue_pending`：待处理 job 数（gauge，按状态 pending/claimed）
- `notice_queue_retry_pending`：等待重试的 job 数
- `notice_success_rate`：区间成功率（可加 alert 用）
- `notice_audit_total{action}`：审计事件计数（可选）

**运行时指标（利于排障）**
- `go_goroutines`、`process_resident_memory_bytes`、`process_cpu_seconds_total`（client_golang 自动提供）
- `http_requests_total{code,method,path}`：访问量（可复用在现有 accessLogger 中间件）

> 现成的数据源：`task_logs`（发送量/成功率/耗时）、`send_jobs`（队列积压）、
> `audit_logs`（审计事件）。可在 service/repository 层加只读统计查询，或直接基于计数器。
> 发送量已具备保留期清理（`LOG_RETENTION_DAYS`），Prometheus 端以自身保留周期为准，
> 两者互不依赖。

### 3.2 作为 Alertmanager 的 Webhook receiver（无需改动）

Alertmanager 原生支持 **Webhook receiver**：向任意 URL `POST` JSON。
notice-service 的 `POST /api/webhook/<api_key>` 已经是公开的异步入队入口（返回 202），
可直接作为接收端，**无需新增代码**。推荐：

- 在渠道管理新建一个专用渠道（如钉钉/企微机器人），模板做一版「监控告警」模板
  （把 Alertmanager 字段映射为友好正文）
- 新建一个 api 任务，绑定该渠道与模板，得到专属 `api_key`，配给 Alertmanager
- 该任务的触发日志会自动记录「触发方式=Webhook、触发 IP=Alertmanager 地址」——复用现有功能

### 3.3 Alertmanager → notice-service 的消息映射

Alertmanager webhook 发送的 JSON 结构（告警组）建议在模板里用变量承接，例如：

```json
{
  "status": "firing",
  "alerts": [{
    "labels": { "alertname": "HighErrorRate", "severity": "critical", "instance": "..." },
    "annotations": { "summary": "..." },
    "startsAt": "...", "endsAt": "..."
  }],
  "groupLabels": {}, "commonAnnotations": {}
}
```

模板变量（变量名与 body 字段对应）：
- `{{status}}`、`{{alerts_count}}`、`{{alert_name}}`、`{{severity}}`、`{{summary}}`、`{{instance}}`
- 实现时 Alertmanager 侧的 webhook 配置需用 `-templates` 或自建小转发器把
  标准 payload 转成 notice-service 的 `{"variables":{...}}` 形态（见 §5 示例）。

## 4. Prometheus 采集与告警规则（示例）

### 4.1 scrape_configs（prometheus.yml）

```yaml
scrape_configs:
  - job_name: notice-service
    metrics_path: /metrics
    # 若开启 Basic Auth / mTLS，配 authorization 或 tls_config（见 §7）
    static_configs:
      - targets: ['127.0.0.1:8080']   # 多实例部署时列全所有节点
```

### 4.2 告警规则（rules/notice.yml，片段）

```yaml
groups:
  - name: notice-service
    rules:
      # 最近 5 分钟发送失败率 > 20%（且样本量足够）
      - alert: NoticeHighFailureRate
        expr: |
          sum(rate(notice_sends_total{status="failed"}[5m]))
            / clamp_min(sum(rate(notice_sends_total[5m])), 1) > 0.2
        for: 5m
        labels: { severity: critical }
        annotations: { summary: "发送失败率过高", description: "5m 失败率 > 20%" }

      # 队列积压持续超过阈值（worker 可能卡死或下游故障）
      - alert: NoticeQueueBacklog
        expr: notice_queue_pending > 100
        for: 10m
        labels: { severity: warning }
        annotations: { summary: "发送队列积压" }

      # 实例离线（up == 0）
      - alert: NoticeInstanceDown
        expr: up{job="notice-service"} == 0
        for: 1m
        labels: { severity: critical }
        annotations: { summary: "notice-service 实例离线" }
```

> 队列本身已有 3 次重试与崩溃接管，告警阈值应高于自愈能力，避免误报：
> 例如「10 分钟内持续 pending > 100」才告警，让重试/接管先发挥兜底作用。

## 5. Alertmanager 路由与 Webhook receiver（示例）

```yaml
route:
  receiver: notice-webhook
  group_by: ['alertname', 'severity']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 1h
receivers:
  - name: notice-webhook
    webhook_configs:
      # 直接转发标准 payload → 若模板字段能直接承接则无需转换；
      # 否则用 -templates 或轻量转发器组装 {"variables":{...}}。
      - url: 'https://your-host/api/webhook/<任务api_key>'
        send_resolved: true
```

若需要把标准 webhook payload 转成 notice-service 的 `{"variables":{...}}`，
可用一个轻量转发函数/脚本（或 Alertmanager `-templates` + `execute_templates`），
示例 payload：

```bash
curl -X POST https://your-host/api/webhook/<api_key> \
  -H 'Content-Type: application/json' \
  -d '{"variables":{"status":"firing","alert_name":"NoticeHighFailureRate","severity":"critical","summary":"5m 失败率 > 20%","instance":"127.0.0.1:8080"}}'
```

## 6. Grafana 与 Zabbix

- **Grafana**：数据源加 Prometheus；建议面板：发送量/成功率趋势（与内置仪表盘互补）、
  队列积压、实例 up 状态、top 失败渠道。可直接在 grafana.com 找现成 Prometheus 面板改。
- **Zabbix**：监控宿主机/容器：进程存活（notice-service 二进制/端口 8080）、磁盘、内存、
  MySQL（notice-service 强依赖库，MySQL 挂了=服务不可用）。Zabbix 的告警可通过
  Alertmanager 的 webhook 或 Zabbix 自身的 webhook media 转发给 notice-service。
  Zabbix 与 Prometheus 是**互补**而非替代：前者管机器，后者管业务指标。

## 7. 安全与注意事项

- **`/metrics` 鉴权**：不要把原始指标直接暴露公网。建议：走内网/反代加
  `allow_connection_from` 或 Basic Auth / mTLS；或与 Webhook 白名单同一套可信代理机制。
- **api_key 保护**：配给 Alertmanager 的 `api_key` 相当于对外发消息的凭据，
  应视为敏感配置；访问日志已对 `/api/webhook/<api_key>` 路径脱敏。
- **IP 白名单**：可给该任务配置 `allowed_ips` 只允许 Alertmanager/Zabbix 主机触发，
  复用现有的可信代理判定（`TRUSTED_PROXIES`）。
- **告警风暴**：依赖 Alertmanager 的 group/repeat 参数；notice-service 每 api_key
  已有 60 次/分钟限流兜底。
- **多实例**：多实例部署时 Prometheus 需采集全部节点；notice-service 的租约锁与队列
  认领保证单实例宕机自动接管，告警规则应容忍这种自愈抖动（`for` 拉长）。
- **保留期**：Prometheus 保留周期独立于 `LOG_RETENTION_DAYS`，长历史查询以
  Prometheus 为准，二者数据源不冲突。

## 8. 实施里程碑（建议顺序）

1. **M1 指标端**：接入 `prometheus/client_golang`，新增受保护的 `/metrics`；
   在 service 层补 `task_logs`/`send_jobs` 的统计查询；Go 测试覆盖关键指标。
2. **M2 采集与可视化**：Prometheus scrape + Grafana 面板；验证多实例拓扑。
3. **M3 告警闭环**：定义告警规则 → Alertmanager 路由 → notice-service 专用
   api 任务 + 监控告警模板 → 端到端打一次真告警验证；检查发送日志记录触发来源。
4. **M4 主机级监控**：接入 Zabbix（进程/资源/MySQL），与业务告警合并路由。
5. **M5 完善**：面板/告警阈值调优、静默维护窗口、`send_resolved` 恢复通知。

## 9. 与现有功能的关系（无需改动即可受益）

- 触发日志已记录「Webhook 告警」的触发方式/触发 IP → 审计可追踪是谁的告警打进来的
- 操作审计记录告警任务的创建/修改
- 2FA 保护管理后台，告警配置不会被未授权者改动
- 多渠道扇出：一条告警可同时发邮件+企微+钉钉，无需重复配置

---

*本文为设计讨论稿，对应代码改动见后续里程碑实施；如需要，可先落地 M1（/metrics 端点）。*

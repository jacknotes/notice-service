package handler

import (
	"log"
	"regexp"
	"strings"
)

// sysErrPattern 命中底层驱动/系统错误特征（表结构、连接细节、驱动栈等）。
// 业务错误均为可读中文文案，不会包含这些标记。
var sysErrPattern = regexp.MustCompile(
	`(?i)(dial tcp|connect: |sql:|mysql|mariadb|driver|duplicate entry|error \d{4}|errno|` +
		`unknown column|doesn't exist|no rows in result set|context deadline|unexpected eof|` +
		`broken pipe|bcrypt|crypto/|base64|json: |yaml:|failed to unmarshal|signature is invalid)`)

const genericErrText = "服务器开小差了，请稍后重试"

// sanitizeErr 对外错误脱敏：业务可读消息原样透出给前端；命中底层驱动/系统
// 特征的错误替换为通用文案，原文仅落服务端日志——避免把表名/列名/连接细节
// 泄漏给客户端。
func sanitizeErr(err error) string {
	if err == nil {
		return genericErrText
	}
	msg := err.Error()
	if msg == "" {
		return genericErrText
	}
	if sysErrPattern.MatchString(msg) {
		log.Printf("handler: suppressed internal error detail: %v", err)
		return genericErrText
	}
	return strings.TrimSpace(msg)
}

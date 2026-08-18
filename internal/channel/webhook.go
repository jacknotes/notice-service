package channel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var webhookClient = &http.Client{Timeout: 15 * time.Second}

func postJSON(url string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := webhookClient.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("http %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func checkWebhookResp(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	if code, ok := m["errcode"].(float64); ok && code != 0 {
		return fmt.Errorf("webhook errcode=%v errmsg=%v", code, m["errmsg"])
	}
	if code, ok := m["code"].(float64); ok && code != 0 {
		return fmt.Errorf("webhook code=%v msg=%v", code, m["msg"])
	}
	return nil
}

// checkCodeResp 校验 code 字段是否等于指定成功值。
// 不同厂商成功码不同：飞书 code=0，PushPlus code=200。
func checkCodeResp(data []byte, successCode int) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	if code, ok := m["code"].(float64); ok && int(code) != successCode {
		return fmt.Errorf("webhook code=%v msg=%v", code, m["msg"])
	}
	return nil
}

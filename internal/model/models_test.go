package model

import (
	"encoding/json"
	"testing"
)

func TestTaskJSONFields(t *testing.T) {
	task := Task{
		ID: 1, UserID: 2, Name: "t", TriggerType: "cron",
		Receivers: []string{"a@x.com"}, CronExpr: "0 9 * * *",
		AllowedIPs: []string{"10.0.0.1"}, Enabled: true,
	}
	b, _ := json.Marshal(task)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"id", "user_id", "name", "trigger_type", "receivers", "cron_expr", "allowed_ips", "enabled"} {
		if _, ok := m[k]; !ok {
			t.Errorf("json field %q missing", k)
		}
	}
	if _, ok := m["locked_by"]; ok {
		t.Error("locked_by should be hidden from json")
	}
}

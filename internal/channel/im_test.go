package channel

import (
	"strings"
	"testing"
)

func TestWechatValidate(t *testing.T) {
	w := &WechatChannel{}
	if err := w.ValidateConfig(map[string]string{"pushplus_token": "t"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing token should fail")
	}
}

func TestWecomValidate(t *testing.T) {
	w := &WecomChannel{}
	if err := w.ValidateConfig(map[string]string{"webhook_url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x"}); err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateConfig(map[string]string{}); err == nil {
		t.Error("missing webhook_url should fail")
	}
}

func TestDingtalkValidateAndSign(t *testing.T) {
	d := &DingtalkChannel{}
	if err := d.ValidateConfig(map[string]string{"webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=x"}); err != nil {
		t.Fatal(err)
	}
	signed := d.signedURL("https://oapi.dingtalk.com/robot/send?access_token=x", "secret", "1627111111111")
	if !strings.Contains(signed, "timestamp=1627111111111") || !strings.Contains(signed, "sign=") {
		t.Errorf("signedURL missing params: %s", signed)
	}
}

func TestFeishuValidate(t *testing.T) {
	f := &FeishuChannel{}
	if err := f.ValidateConfig(map[string]string{"webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/x"}); err != nil {
		t.Fatal(err)
	}
}

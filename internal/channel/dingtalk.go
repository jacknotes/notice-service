package channel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

type DingtalkChannel struct {
	config map[string]string
}

func (d *DingtalkChannel) Type() string { return "dingtalk" }

func (d *DingtalkChannel) ValidateConfig(c map[string]string) error {
	if c["webhook_url"] == "" {
		return fmt.Errorf("缺少配置: webhook_url")
	}
	return nil
}

func (d *DingtalkChannel) signedURL(webhookURL, secret, timestamp string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return webhookURL + "&timestamp=" + timestamp + "&sign=" + sign
}

func (d *DingtalkChannel) TestConnection(c map[string]string) error {
	if err := d.ValidateConfig(c); err != nil {
		return err
	}
	return nil
}

func (d *DingtalkChannel) Send(message *Message, receiver *Receiver) error {
	if message == nil || receiver == nil {
		return errors.New("message/receiver 不能为空")
	}
	u := d.config["webhook_url"]
	if sec := d.config["secret"]; sec != "" {
		u = d.signedURL(u, sec, strconv.FormatInt(nowUnix(), 10))
	}
	data, err := postJSON(u, map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": message.Subject,
			"text":  fmt.Sprintf("### %s\n\n%s", message.Subject, message.Content),
		},
	})
	if err != nil {
		return err
	}
	return checkWebhookResp(data)
}

func NewDingtalkChannel(config map[string]string) *DingtalkChannel {
	return &DingtalkChannel{config: config}
}

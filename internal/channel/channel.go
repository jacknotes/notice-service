package channel

// Message 发送内容。
type Message struct {
	Subject string
	Content string
	Extra   map[string]string
}

// Receiver 接收者。
type Receiver struct {
	Address string
}

// Channel 所有通知渠道的统一接口。
type Channel interface {
	Type() string
	ValidateConfig(config map[string]string) error
	TestConnection(config map[string]string) error
	Send(message *Message, receiver *Receiver) error
}

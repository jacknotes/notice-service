package channel

import "sync"

var (
	regMu sync.RWMutex
	reg   = map[string]Channel{}
)

func Register(c Channel) {
	regMu.Lock()
	defer regMu.Unlock()
	reg[c.Type()] = c
}

func Get(t string) (Channel, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := reg[t]
	return c, ok
}

func Types() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(reg))
	for k := range reg {
		out = append(out, k)
	}
	return out
}

func init() {
	Register(NewEmailChannel(nil))
	Register(NewWecomChannel(nil))
	Register(NewDingtalkChannel(nil))
	Register(NewFeishuChannel(nil))
	Register(NewWechatChannel(nil))
}

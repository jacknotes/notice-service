package channel

import "testing"

func TestRegistryRegisterAndGet(t *testing.T) {
	Register(&emailChannel{})
	c, ok := Get("email")
	if !ok {
		t.Fatal("Get(email) should be ok")
	}
	if c.Type() != "email" {
		t.Errorf("Type = %q", c.Type())
	}
	if _, ok := Get("nope"); ok {
		t.Error("unknown channel should not exist")
	}
}

type emailChannel struct{}

func (e *emailChannel) Type() string                             { return "email" }
func (e *emailChannel) ValidateConfig(c map[string]string) error { return nil }
func (e *emailChannel) TestConnection(c map[string]string) error { return nil }
func (e *emailChannel) Send(m *Message, r *Receiver) error       { return nil }

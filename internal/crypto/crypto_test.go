package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := `{"password":"s3cret","token":"abc"}`
	enc, err := c.EncryptString(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Error("ciphertext must differ from plaintext")
	}
	dec, err := c.DecryptString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Errorf("roundtrip = %q, want %q", dec, plain)
	}
}

func TestWrongKeyFails(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	k2[0] = 1
	c1, _ := New(k1)
	c2, _ := New(k2)
	enc, _ := c1.EncryptString("x")
	if _, err := c2.DecryptString(enc); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

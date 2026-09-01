package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHandleCLIArgsHelp: --help/-h 打印用法且不继续启动（handled=true）。
func TestHandleCLIArgsHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		var out, errb bytes.Buffer
		code, handled := handleCLIArgs([]string{arg}, &out, &errb)
		if !handled || code != 0 {
			t.Fatalf("%s: handled=%v code=%d, want handled=true code=0", arg, handled, code)
		}
		if !strings.Contains(out.String(), "Usage: notice-service") {
			t.Fatalf("%s: output missing usage, got %q", arg, out.String())
		}
		if errb.Len() != 0 {
			t.Fatalf("%s: stderr should be empty, got %q", arg, errb.String())
		}
	}
}

// TestHandleCLIArgsVersion: --version/-v 打印版本且不继续启动。
func TestHandleCLIArgsVersion(t *testing.T) {
	for _, arg := range []string{"--version", "-v"} {
		var out, errb bytes.Buffer
		code, handled := handleCLIArgs([]string{arg}, &out, &errb)
		if !handled || code != 0 {
			t.Fatalf("%s: handled=%v code=%d, want handled=true code=0", arg, handled, code)
		}
		if !strings.Contains(out.String(), buildVersion) {
			t.Fatalf("%s: output missing version %q, got %q", arg, buildVersion, out.String())
		}
	}
}

// TestHandleCLIArgsUnknown: 未知参数报错退出（handled=true code=2）。
func TestHandleCLIArgsUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	code, handled := handleCLIArgs([]string{"--bogus"}, &out, &errb)
	if !handled || code != 2 {
		t.Fatalf("unknown: handled=%v code=%d, want handled=true code=2", handled, code)
	}
	if !strings.Contains(errb.String(), "未知参数") {
		t.Fatalf("unknown: stderr missing error, got %q", errb.String())
	}
}

// TestHandleCLIArgsNormalStart: 无参数 / --config / reset-password 应继续启动（handled=false）。
func TestHandleCLIArgsNormalStart(t *testing.T) {
	cases := [][]string{
		{},
		{"--config", "/tmp/x.yml"},
		{"--config=/tmp/x.yml"},
		{"reset-password"},
	}
	for _, args := range cases {
		var out, errb bytes.Buffer
		code, handled := handleCLIArgs(args, &out, &errb)
		if handled {
			t.Fatalf("args %v: handled=true, want false (should start normally)", args)
		}
		if code != 0 {
			t.Fatalf("args %v: code=%d, want 0", args, code)
		}
	}
}

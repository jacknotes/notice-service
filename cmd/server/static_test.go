package main

import (
	"path/filepath"
	"testing"
)

func TestResolveStaticDir(t *testing.T) {
	dir := t.TempDir()
	// 配置路径存在 → 优先
	if got, ok := resolveStaticDir(dir); !ok || got != dir {
		t.Fatalf("configured dir not honored: %q ok=%v", got, ok)
	}
	// 配置路径不存在 → 回退 ./web/dist；测试 CWD 无 web/dist → 再回退可执行文件目录 → 均无 → not ok
	if _, ok := resolveStaticDir(filepath.Join(dir, "nope")); ok {
		t.Fatal("missing dir should report not found")
	}
}

func TestDirExists(t *testing.T) {
	if dirExists(t.TempDir()) != true {
		t.Fatal("existing dir should be true")
	}
	if dirExists(filepath.Join(t.TempDir(), "nope")) != false {
		t.Fatal("missing dir should be false")
	}
	if dirExists("/dev/null") != false {
		t.Fatal("regular file should be false")
	}
}

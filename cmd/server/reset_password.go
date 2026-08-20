package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"

	"notice-service/internal/repository"
	"notice-service/internal/service"
)

// resetPassword 离线重置指定用户的密码：强度校验 + bcrypt + 落库。
// 用户不存在返回 repository.ErrNotFound；密码不达标返回具体错误。
func resetPassword(db *sql.DB, username, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("用户名不能为空")
	}
	hash, err := service.HashPassword(newPassword)
	if err != nil {
		return err
	}
	users := repository.NewUserRepo(db)
	u, err := users.GetByUsername(username)
	if err != nil {
		return err
	}
	return users.UpdatePassword(u.ID, hash)
}

// promptNewPassword 读取新密码：交互式终端隐藏回显；非 TTY（管道/脚本）从 stdin 读取一行。
func promptNewPassword(in io.Reader, out io.Writer) (string, error) {
	if f, ok := in.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(out, "请输入新密码: ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return "", errors.New("未读取到密码")
	}
	return strings.TrimSpace(sc.Text()), sc.Err()
}

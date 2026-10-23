#!/usr/bin/env python3
"""最小假 SMTP 服务器（用于验证 H-2 认证机制协商），不实现 TLS。

用法：
  python3 fake_smtp.py <port> <AUTH机制列表，空格分隔>
例：
  python3 fake_smtp.py 2525 "CRAM-MD5 LOGIN PLAIN"   # 服务器宣告三种机制
  python3 fake_smtp.py 2526 "LOGIN PLAIN"            # 只宣告 LOGIN/PLAIN

服务端会把客户端实际使用的 AUTH 机制打印到 stdout（MECh=<名字>），
这正是 H-2 要观察的行为：非 TLS 且宣告了 CRAM-MD5 时必须选 CRAM-MD5，
而不是因为服务器把 LOGIN 排前面就选 LOGIN。
"""
import base64
import hashlib
import hmac
import socket
import sys
import threading

PORT = int(sys.argv[1])
MECHS = sys.argv[2] if len(sys.argv) > 2 else "CRAM-MD5 LOGIN PLAIN"


def handle(conn, addr):
    f = conn.makefile("rwb", buffering=0)

    def send(line):
        f.write((line + "\r\n").encode())

    send("220 fake-smtp ready")
    challenge = b"<1896.697170952@fake>"
    while True:
        raw = f.readline()
        if not raw:
            return
        line = raw.decode("utf-8", "replace").strip()
        if not line:
            continue
        up = line.upper()

        if up.startswith("EHLO") or up.startswith("HELO"):
            send("250-fake-smtp")
            # 注意：不宣告 STARTTLS，制造「非 TLS」场景
            send("250-AUTH " + MECHS)
            send("250 OK")
        elif up.startswith("AUTH "):
            parts = line.split()
            mech = parts[1].upper() if len(parts) > 1 else ""
            print("MECH=" + mech, flush=True)
            if mech == "CRAM-MD5":
                send("334 " + base64.b64encode(challenge).decode())
                resp = f.readline().decode().strip()
                try:
                    decoded = base64.b64decode(resp).decode()
                    user, digest = decoded.split(" ", 1)
                    print("CRAM_USER=" + user, flush=True)
                    print("CRAM_DIGEST=" + digest, flush=True)
                except Exception as e:  # noqa: BLE001
                    print("CRAM_PARSE_ERR=" + str(e), flush=True)
                send("235 auth ok")
            elif mech == "LOGIN":
                send("334 " + base64.b64encode(b"Username:").decode())
                f.readline()
                send("334 " + base64.b64encode(b"Password:").decode())
                pw = f.readline().decode().strip()
                print("LOGIN_PASSWORD_B64=" + pw, flush=True)
                send("235 auth ok")
            elif mech == "PLAIN":
                send("334 ")
                resp = f.readline().decode().strip()
                print("PLAIN_B64=" + resp, flush=True)
                send("235 auth ok")
            else:
                send("504 unsupported mechanism")
        elif up.startswith("MAIL FROM") or up.startswith("RCPT TO"):
            send("250 OK")
        elif up == "DATA":
            send("354 send data")
            while True:
                d = f.readline()
                if not d or d.strip() == b".":
                    break
            send("250 OK queued")
        elif up == "QUIT":
            send("221 bye")
            return
        else:
            send("250 OK")


srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
srv.bind(("127.0.0.1", PORT))
srv.listen(5)
print("LISTEN port=%d mechs=%s" % (PORT, MECHS), flush=True)
while True:
    c, a = srv.accept()
    threading.Thread(target=handle, args=(c, a), daemon=True).start()

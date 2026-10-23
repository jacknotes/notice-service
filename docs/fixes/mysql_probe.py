"""用 PyMySQL 协议直连验证 Navicat 账号是否可用（不依赖 mysql 客户端）。

用法：
  python mysql_probe.py <host> <port> <user> <password> <database>
"""
import hashlib
import socket
import struct
import sys

host, port, user, password, database = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4], sys.argv[5]

CLIENT_LONG_PASSWORD = 0x00000001
CLIENT_LONG_FLAG = 0x00000004
CLIENT_CONNECT_WITH_DB = 0x00000008
CLIENT_PROTOCOL_41 = 0x00000200
CLIENT_TRANSACTIONS = 0x00002000
CLIENT_SECURE_CONNECTION = 0x00008000
CLIENT_PLUGIN_AUTH = 0x00080000

caps = (CLIENT_LONG_PASSWORD | CLIENT_LONG_FLAG | CLIENT_CONNECT_WITH_DB |
        CLIENT_PROTOCOL_41 | CLIENT_TRANSACTIONS | CLIENT_SECURE_CONNECTION | CLIENT_PLUGIN_AUTH)


def read_packet(sock):
    hdr = b""
    while len(hdr) < 4:
        b = sock.recv(4 - len(hdr))
        if not b:
            raise EOFError
        hdr += b
    length = hdr[0] | (hdr[1] << 8) | (hdr[2] << 16)
    payload = b""
    while len(payload) < length:
        b = sock.recv(length - len(payload))
        if not b:
            raise EOFError
        payload += b
    return payload


def scramble_native(pw, salt):
    if not pw:
        return b""
    s1 = hashlib.sha1(pw.encode()).digest()
    s2 = hashlib.sha1(s1).digest()
    s3 = hashlib.sha1(salt[:20] + s2).digest()
    return bytes(a ^ b for a, b in zip(s1, s3))


sock = socket.create_connection((host, port), timeout=10)
greet = read_packet(sock)
# 解析 server greeting 取 salt
i = 0
i += 1  # protocol version
end = greet.index(b"\0", i)
i = end + 1
i += 4  # thread id
salt1 = greet[i:i + 8]  # auth-plugin-data-part-1（8 字节）
i += 8
i += 1  # filler
i += 2  # caps lower
i += 1  # charset
i += 2  # status
i += 2  # caps upper
i += 1  # auth data len
i += 10  # reserved
salt2 = greet[i:i + 12].rstrip(b"\0")  # auth-plugin-data-part-2
i += 13
salt = salt1 + salt2  # mysql_native_password 用 20 字节盐
auth_name = greet[i:].split(b"\0")[0].decode(errors="replace")

auth = scramble_native(password, salt)
payload = struct.pack("<I", caps) + struct.pack("<I", 16 * 1024 * 1024)
payload += bytes([33]) + b"\0" * 23
payload += user.encode() + b"\0"
payload += bytes([len(auth)]) + auth
payload += database.encode() + b"\0"
payload += b"mysql_native_password\0"

hdr = struct.pack("<I", len(payload))[:3] + b"\x01"
sock.sendall(hdr + payload)
resp = read_packet(sock)
code = resp[0]
if code == 0x00:
    print("OK login as %s@%s db=%s (server auth=%s)" % (user, host, database, auth_name))
    sock.close()
    sys.exit(0)
elif code == 0xFF:
    errno = struct.unpack("<H", resp[1:3])[0]
    msg = resp[3:].decode(errors="replace")
    print("FAIL errno=%d %s" % (errno, msg))
    sock.close()
    sys.exit(1)
else:
    print("UNEXPECTED first byte=0x%02x" % code)
    sock.close()
    sys.exit(2)

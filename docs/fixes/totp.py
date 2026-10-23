#!/usr/bin/env python3
"""生成 TOTP 6 位动态码（RFC 6238，与 internal/totp 同算法）。

用法：
  python3 totp.py <BASE32_SECRET>
例：
  python3 totp.py KWPJSK7OAZHDZKA7R6GCGXK3GVN5VHJX
"""
import base64
import hashlib
import hmac
import struct
import sys
import time

secret = sys.argv[1].strip().replace(" ", "").upper()
pad = "=" * ((8 - len(secret) % 8) % 8)
key = base64.b32decode(secret + pad)
counter = int(time.time()) // 30
msg = struct.pack(">Q", counter)
digest = hmac.new(key, msg, hashlib.sha1).digest()
offset = digest[-1] & 0x0F
code = (struct.unpack(">I", digest[offset:offset + 4])[0] & 0x7FFFFFFF) % 1000000
print("%06d" % code)

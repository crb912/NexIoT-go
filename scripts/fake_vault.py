from http.server import BaseHTTPRequestHandler, HTTPServer
import json


# mkdir -p /tmp/edgex/secrets/device-simple/
#

class FakeVaultHandler(BaseHTTPRequestHandler):
    # 屏蔽默认的终端请求日志，保持控制台干净
    def log_message(self, format, *args):
        print(f"[Fake Vault] 收到请求: {self.command} {self.path}")

    def do_GET(self):
        # 默认返回 200 OK
        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.end_headers()

        # 1. 模拟 Vault 健康检查 (EdgeX 启动时通常会检查 Vault 是否准备就绪)
        if '/v1/sys/health' in self.path:
            response = {
                "initialized": True,
                "sealed": False,     # 告诉 Go SDK: 我没有被封锁，可以读取
                "standby": False
            }

        # 2. 模拟 EdgeX 请求特定的 Secret (根据你的配置: RequiredSecrets = "redisdb")
        # 注意：不同框架请求的路径可能略有不同，通常带有 /v1/secret/
        elif '/v1/secret/data/device-simple' in self.path or '/v1/secret/' in self.path:
            # 返回伪造的密钥数据 (严格按照 Vault KV v2 引擎的 JSON 格式)
            response = {
                "data": {
                    "username": "default",
                    "password": "fake_redis_password",
                    "redisdb": "redis://:fake_redis_password@localhost:6379"
                }
            }

        # 3. 模拟 Token 验证逻辑 (如果 SDK 发送了 lookup-self)
        elif '/v1/auth/token/lookup-self' in self.path:
            response = {
                "data": {
                    "id": "my_test_token",
                    "policies": ["root"]
                }
            }

        else:
            # 兜底回复
            response = {"msg": "mocked"}

        # 将 Python 字典转为 JSON 字符串并返回给 Go SDK
        self.wfile.write(json.dumps(response).encode('utf-8'))

def run(port=8200):
    server_address = ('127.0.0.1', port)
    httpd = HTTPServer(server_address, FakeVaultHandler)
    print(f"🚀 纯 Python 伪造的 Vault 服务器已启动，监听端口: {port}")
    print("等待 Go SDK 连接... (按 Ctrl+C 停止)")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n已关闭 Fake Vault。")

if __name__ == '__main__':
    run()
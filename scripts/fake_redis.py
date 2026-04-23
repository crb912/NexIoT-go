import redislite
import time

print("正在启动带密码保护的 Redis 服务器...")

# 1. 定义服务器的配置
config = {
    'port': '6379',              # 让它监听 6379 端口
    'requirepass': 'fake_redis_password' # [关键点] 设置 Redis 服务器的密码
}

# 2. 启动服务，并创建客户端实例
# - serverconfig=config: 告诉底层服务器使用上面的配置启动
# - password='my_password': 告诉当前的 Python 客户端用这个密码去连接自己刚启动的服务器
r = redislite.Redis(serverconfig=config, password='fake_redis_password')

# 测试一下能不能正常存取
r.set('test_key', 'Hello Authenticated Redis!')
print("内部测试读取:", r.get('test_key').decode('utf-8'))

print("-" * 40)
print("服务器已启动并受密码保护！监听 localhost:6379")
print("其他脚本现在必须提供密码 'fake_redis_password' 才能连接。按 Ctrl+C 停止。")
print("-" * 40)

try:
    # 阻塞脚本，让服务器持续在后台运行
    while True:
        time.sleep(1)
except KeyboardInterrupt:
    print("\n服务器已关闭。")
#!/bin/bash

# 获取本机IP地址
# 这里使用ip命令，适用于大多数Linux发行版
# 如果你的系统使用的是其他命令，请相应修改
ip_address=$(ip addr show eth0 | grep "inet\b" | awk '{print $2}' | cut -d/ -f1)

# 检查data目录是否存在，如果不存在则创建
if [ ! -d "data" ]; then
    mkdir data
fi

# 定义端口号
port=50061

# 将IP地址和端口号写入data目录下的snaddr文件中
echo -e "$ip_address:$port" > data/snaddr

# 打印一条消息表示操作完成
echo "IP地址已写入data/snaddr文件中。"
#!/bin/bash

# 获取外部传入的snNum参数
if [ -z "$1" ]; then
  echo "Error: Number of SNs not provided"
  exit 1
else
  snNum=$1
fi

# 定义snips文件的路径
SNIPS_FILE="/home/ubuntu/ECDS/data/snips"

# 定义SSH密码
SSH_PASSWORD="jjp918JJP"

# 定义目标日志文件路径
REMOTE_LOG_FILE="/home/ubuntu/ECDS/data/output_sn.log"

# 定义本地日志文件路径
LOCAL_LOG_FILE="/home/ubuntu/ECDS/data/output_sn.log"

# # 持续监测snips文件中的每个IP地址
# while true; do
    # 清空当前的本地日志文件
    > "$LOCAL_LOG_FILE"

    # 遍历 snaddrs 文件中的每一行 IP 地址
    for (( i=1; i<=$snNum; i++ ))
    do
        # 读取每个 IP 地址
        ip_addr=$(sed -n "${i}p" $SNIPS_FILE)
        # 检查IP地址是否有效
        if [[ $ip_addr =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "Valid IP address: $ip_addr"
            # 从目标主机同步日志文件并追加到本地日志文件
            rsync -avz -e "sshpass -p $SSH_PASSWORD ssh -o StrictHostKeyChecking=no" "ubuntu@$ip_addr:$REMOTE_LOG_FILE" "$LOCAL_LOG_FILE.tmp" && cat "$LOCAL_LOG_FILE.tmp" >> "$LOCAL_LOG_FILE" && rm "$LOCAL_LOG_FILE.tmp"
            echo "Log from $ip_addr has been appended to local log."
        else
            echo "Invalid IP address: $ip_addr"
        fi
    done
    
#     # 等待一段时间（例如60秒）再次执行
#     sleep 2
# done
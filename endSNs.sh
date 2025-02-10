#!/bin/bash

# 检查第一个参数是否为空
if [ -z "$1" ]; then
    snmNum=31
else
    snmNum=$1
fi

# 检查第二个参数是否为空
if [ -z "$2" ]; then
    end_keyword="exp_sn"
else
    end_keyword=$2
fi

# 定义snips文件的路径
# SNIPS_FILE="/home/ubuntu/ECDS/data/snips"
SNIPS_FILE="/root/DSN/ECDS/data/snips"


# 定义SSH密码
# SSH_PASSWORD="jjp918JJP"
SSH_PASSWORD="bassword"

# 检查snips文件是否存在
if [ ! -f "$SNIPS_FILE" ]; then
  echo "Error: snips file does not exist at $SNIPS_FILE"
  exit 1
fi

# 遍历 snaddrs 文件中的每一行 IP 地址
for (( i=1; i<=$snmNum; i++ ))
do
    # 读取每个 IP 地址
    ip_addr=$(sed -n "${i}p" $SNIPS_FILE)

    # 检查 IP 地址是否有效
    if [[ $ip_addr =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Valid IP address: $ip_addr"

        # 使用sshpass和ssh命令结束远程主机上的脚本
        # if sshpass -p "$SSH_PASSWORD" ssh ubuntu@"$ip_addr" "pkill -f $end_keyword"; then
        if sshpass -p "$SSH_PASSWORD" ssh root@"$ip_addr" "pkill -f $end_keyword"; then
            echo "Process $end_keyword on $ip_addr has been terminated."
        else
            echo "Failed to terminate script $end_keyword on $ip_addr."
        fi

    else
        echo "Invalid IP address: $ip_addr"
    fi
done

echo "All tasks completed."
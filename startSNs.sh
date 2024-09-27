#!/bin/bash

# 检查第一个参数是否为空
if [ -z "$1" ]; then
    snNum=2
else
    snNum=$1
fi

# 检查第二个参数是否为空
if [ -z "$2" ]; then
    SCRIPT_NAME="exp_putfile_sn.sh"
else
    SCRIPT_NAME=$2
fi

# 检查第二个参数是否为空
if [ -z "$3" ]; then
    dsnMode="ec"
else
    dsnMode=$3
fi

# 定义源脚本路径
SCRIPT_PATH="/home/ubuntu/ECDS/expsh/"

# 定义snips文件的路径
SNIPS_FILE="/home/ubuntu/ECDS/data/snips"

# 定义SSH密码
SSH_PASSWORD="jjp918JJP"

# 检查snips文件是否存在
if [ ! -f "$SNIPS_FILE" ]; then
  echo "Error: snips file does not exist at $SNIPS_FILE"
  exit 1
fi

# 启动snlog同步脚本
bash syncSNLog.sh "$snNum"

# 遍历 snaddrs 文件中的每一行 IP 地址
for (( i=1; i<=$snNum; i++ ))
do
    # 读取每个 IP 地址
    ip_addr=$(sed -n "${i}p" $SNIPS_FILE)

    # 检查 IP 地址是否有效
    if [[ $ip_addr =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Valid IP address: $ip_addr"

        # 使用sshpass复制文件到目标主机
        if ! sshpass -p "$SSH_PASSWORD" scp "$SCRIPT_PATH$SCRIPT_NAME" ubuntu@$ip_addr:"$SCRIPT_PATH"; then
          echo "Failed to copy script to $ip_addr"
          continue
        fi

        # 使用sshpass在目标主机上给予脚本执行权限
        if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$ip_addr "chmod +x $SCRIPT_PATH$SCRIPT_NAME"; then
          echo "Failed to set execute permission on $ip_addr"
          continue
        fi

        # 使用sshpass在目标主机上执行脚本
        if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$ip_addr "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode; then
          echo "Failed to execute script on $ip_addr"
        else
          echo "Script executed successfully on $ip_addr"
        fi
    else
        echo "Invalid IP address: $ip_addr"
    fi
done

sleep $((snNum * 5))

bash syncSNLog.sh "$snNum"

echo "All tasks completed."
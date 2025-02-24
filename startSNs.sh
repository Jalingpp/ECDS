#!/bin/bash

# 检查第一个参数是否为空
if [ -z "$1" ]; then
    snNum=4
else
    snNum=$1
fi

# 检查第二个参数是否为空
if [ -z "$2" ]; then
    SCRIPT_NAME="exp_sn.sh"
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
# SCRIPT_PATH="/home/ubuntu/ECDS/expsh/"
SCRIPT_PATH="/root/ECDS/expsh/"

# 定义snips文件的路径
# SNIPS_FILE="/home/ubuntu/ECDS/data/snaddrs"
SNIPS_FILE="/root/ECDS/data/snaddrs"

# 定义SSH密码
# SSH_PASSWORD="jjp918JJP"
SSH_PASSWORD="bassword"

# 检查snips文件是否存在
if [ ! -f "$SNIPS_FILE" ]; then
  echo "Error: snips file does not exist at $SNIPS_FILE"
  exit 1
fi

# # 启动snlog同步脚本
# bash syncSNLog.sh "$snNum"

# 遍历 snaddrs 文件中的每一行 IP 地址
for (( i=1; i<=$snNum; i++ ))
do
    # 使用awk命令提取第i行的IP地址和端口号
    dataline=$(head -n $i $SNIPS_FILE | tail -n 1)
    IP_PORT=$(echo $dataline | cut -d ',' -f2)

    # 将IP地址和端口号分解为两个变量
    IP=$(echo $IP_PORT | cut -d ':' -f1)
    PORT=$(echo $IP_PORT | cut -d ':' -f2)

    # 检查 IP 地址是否有效
    if [[ $IP =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Valid IP address: $IP"

        # 使用sshpass复制文件到目标主机
        # if ! sshpass -p "$SSH_PASSWORD" scp "$SCRIPT_PATH$SCRIPT_NAME" ubuntu@$IP:"$SCRIPT_PATH"; then
        if ! sshpass -p "$SSH_PASSWORD" scp -P 22001 "$SCRIPT_PATH$SCRIPT_NAME" root@$IP:"$SCRIPT_PATH"; then
          echo "Failed to copy script to $IP"
          continue
        fi

        # 使用sshpass在目标主机上给予脚本执行权限
        # if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$IP "chmod +x $SCRIPT_PATH$SCRIPT_NAME"; then
        if ! sshpass -p "$SSH_PASSWORD" ssh -tt -p 22001 root@$IP "chmod +x $SCRIPT_PATH$SCRIPT_NAME"; then
          echo "Failed to set execute permission on $IP"
          continue
        fi

        # 使用sshpass在目标主机上执行脚本
        # if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$IP "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode $PORT; then
        if ! sshpass -p "$SSH_PASSWORD" ssh -tt -p 22001 root@$IP "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode $PORT; then
          echo "Failed to execute script on $IP"
        else
          echo "Script executed successfully on $IP"
        fi
    else
        echo "Invalid IP address: $IP"
    fi
done

sleep $((snNum * 5))
# sleep 5

# bash syncSNLog.sh "$snNum"

echo "All tasks completed."
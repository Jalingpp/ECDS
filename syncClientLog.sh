# 检查第一个参数是否为空
if [ -z "$1" ]; then
    CLIENT_IP="10.24.15.34"
else
    CLIENT_IP=$1
fi

# 定义SSH密码
SSH_PASSWORD="jjp918JJP"

# 定义目标日志文件路径
REMOTE_LOG_FILE="/home/ubuntu/ECDS/data/output_client.log"

# 定义本地日志文件路径
LOCAL_LOG_FILE="/home/ubuntu/ECDS/data/output_client.log"

# 清空当前的本地日志文件
> "$LOCAL_LOG_FILE"

# 从目标主机同步日志文件并追加到本地日志文件
rsync -avz -e "sshpass -p $SSH_PASSWORD ssh -o StrictHostKeyChecking=no" "ubuntu@$CLIENT_IP:$REMOTE_LOG_FILE" "$LOCAL_LOG_FILE.tmp" && cat "$LOCAL_LOG_FILE.tmp" >> "$LOCAL_LOG_FILE" && rm "$LOCAL_LOG_FILE.tmp"
echo "Log from $CLIENT_IP has been appended to local log."
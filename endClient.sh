# 检查第一个参数是否为空
if [ -z "$1" ]; then
    CLIENT_IP="10.24.15.34"
else
    CLIENT_IP=$1
fi

# 检查第二个参数是否为空
if [ -z "$2" ]; then
    end_keyword="exp_putfile_client"
else
    end_keyword=$2
fi

# 定义SSH密码
SSH_PASSWORD="jjp918JJP"

# 使用sshpass和ssh命令结束远程主机上的脚本
if sshpass -p "$SSH_PASSWORD" ssh ubuntu@"$CLIENT_IP" "pkill -f $end_keyword"; then
  echo "Process $end_keyword on $CLIENT_IP has been terminated."
else
  echo "Failed to terminate script $end_keyword on $CLIENT_IP."
fi
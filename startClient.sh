
# 检查第一个参数是否为空
if [ -z "$1" ]; then
    CLIENT_IP="172.23.165.211"
else
    CLIENT_IP=$1
fi

# 检查第三个参数是否为空
if [ -z "$2" ]; then
    SCRIPT_NAME="exp_client.sh"
else
    SCRIPT_NAME=$2
fi

# 检查第一个参数是否为空
if [ -z "$3" ]; then
    dsnMode="ec"
else
    dsnMode=$3
fi

# 检查第一个参数是否为空
if [ -z "$4" ]; then
    clientnum=4
else
    clientnum=$4
fi

# 检查第一个参数是否为空
if [ -z "$5" ]; then
    datafiledir="/home/ubuntu/ECDS/data/NM/"
else
    datafiledir=$5
fi

# 检查第一个参数是否为空
if [ -z "$6" ]; then
    datafilenum=1
else
    datafilenum=$6
fi

# 定义源脚本路径
# SCRIPT_PATH="/home/ubuntu/ECDS/expsh/"
SCRIPT_PATH="/root/ECDS/expsh/"

# 定义SSH密码
# SSH_PASSWORD="jjp918JJP"
SSH_PASSWORD="bassword"

# # 启动clientlog同步脚本
# bash syncClientLog.sh "$CLIENT_IP"

# 使用sshpass复制文件到目标主机
# if ! sshpass -p "$SSH_PASSWORD" scp "$SCRIPT_PATH$SCRIPT_NAME" ubuntu@$CLIENT_IP:"$SCRIPT_PATH"; then
if ! sshpass -p "$SSH_PASSWORD" scp -P 22008 "$SCRIPT_PATH$SCRIPT_NAME" root@$CLIENT_IP:"$SCRIPT_PATH"; then
  echo "Failed to copy script to $CLIENT_IP"
  continue
fi

# 使用sshpass在目标主机上给予脚本执行权限
# if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$CLIENT_IP "chmod +x $SCRIPT_PATH$SCRIPT_NAME"; then
if ! sshpass -p "$SSH_PASSWORD" ssh -tt -p 22008 root@$CLIENT_IP "chmod +x $SCRIPT_PATH$SCRIPT_NAME"; then
  echo "Failed to set execute permission on $CLIENT_IP"
  continue
fi

# 使用sshpass在目标主机上执行脚本
# if ! sshpass -p "$SSH_PASSWORD" ssh -tt ubuntu@$CLIENT_IP "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode $clientnum $datafiledir $datafilenum; then
if ! sshpass -p "$SSH_PASSWORD" ssh -tt -p 22008 root@$CLIENT_IP "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode $clientnum $datafiledir $datafilenum; then
  echo "Failed to execute script on $CLIENT_IP"
else
  echo "Script executed successfully on $CLIENT_IP"
fi

# # 启动clientlog同步脚本
# bash syncClientLog.sh "$CLIENT_IP"
#!/bin/bash
# 该脚本实现：1.根据snips写所有SN的snaddrs文件(同一个ip对应多个port)；2.写AC本地snaadrs文件；3.复制AC的snaddrs文件到Client。

# AC 的 IP 地址
AC_IP="10.0.4.10"

# Client 的 IP 地址
Client_IP="10.0.4.11"

# snNum 的数量（假设已经定义）
snNum=31

# 运行存储服务节点的机器数量（即snips中ip个数）
ipNum=1
# 每个ip对应的要生成ip:port的数量
portNums=(31)

# 包含 IP 地址的文件
SN_ADDRS_FILE="/home/ubuntu/ECDS/data/snips"
# SN_ADDRS_FILE="/root/DSN/ECDS/data/snips"

# 要写入远端文件的字符串的前缀
prefix="sn"

# 密码
PASSWORD="jjp918JJP"
# PASSWORD="bassword"

# 确保 snips 文件存在
if [ ! -f "$SN_ADDRS_FILE" ]; then
    echo "snips 文件不存在"
    exit 1
fi

# 清空当前的 snaddrs 文件（如果存在）
rm -f /home/ubuntu/ECDS/data/snaddrs
# rm -f /root/DSN/ECDS/data/snaddrs

startport=50061
snid=0
# 遍历snaddrs 文件中的每一行 IP 地址，用每个IP生成一系列snaddr
for (( i=1; i<=$ipNum; i++ ))
do
    # 读取snips文件中的IP地址
    ip_addr=$(head -n $i $SN_ADDRS_FILE | tail -n 1)
    # 为每个IP地址生成snaddrs
    portNum=${portNums[$((i - 1))]}
    for (( j=1; j<=portNum; j++ ))
    do
        port=$(( startport + j - 1 ))
        snid=$((snid+1))
        # 构建要写入的字符串
        data="${prefix}${snid},${ip_addr}:${port}"
        echo "将要写入的数据: $data"

        # 使用 sshpass 连接到每个 IP 地址并写入文件
        sshpass -p "$PASSWORD" ssh ubuntu@$ip_addr "echo '$data' > /home/ubuntu/ECDS/data/snaddrfile/${port}"
        # sshpass -p "$PASSWORD" ssh ubuntu@$ip_addr "echo '$data' > /root/DSN/ECDS/data/snaddrfile/${port}"
        
        # 将数据追加到本地文件
        echo "$data" >> /home/ubuntu/ECDS/data/snaddrs
        # echo "$data" >> /root/DSN/ECDS/data/snaddrs

        # 检查 SSH 命令是否成功执行
        if [ $? -eq 0 ]; then
            echo "成功写入 $ip_addr 的 /home/ubuntu/ECDS/data/snaddrfile/${port} 文件"
            # echo "成功写入 $ip_addr 的 /root/DSN/ECDS/data/snaddrfile/${port} 文件"
        else
            echo "写入 $ip_addr 的 /home/ubuntu/ECDS/data/snaddrfile/${port} 文件失败"
            # echo "写入 $ip_addr 的 /root/DSN/ECDS/data/snaddrfile/${port} 文件失败"
        fi
    done
done

ssh-keyscan -H $Client_IP >> ~/.ssh/known_hosts

# 复制文件到Client
if sshpass -p "$PASSWORD" scp /home/ubuntu/ECDS/data/snaddrs ubuntu@$Client_IP:/home/ubuntu/ECDS/data; then
# if sshpass -p "$PASSWORD" scp /root/DSN/ECDS/data/snaddrs ubuntu@$Client_IP:/root/DSN/ECDS/data/; then
    echo "文件成功复制到 Client"
else
    echo "文件复制到 Client 失败"
fi

echo "所有数据已写入 AC 的 /home/ubuntu/ECDS/data/snaddrs 文件，并已复制到Client"
# echo "所有数据已写入 AC 的 /root/DSN/ECDS/data/snaddrs 文件，并已复制到Client"
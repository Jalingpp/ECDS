#!/bin/bash
# 该脚本实现：1.根据snips写所有SN的snaddrs文件(同一个ip对应一个port)；2.写AC本地snaadrs文件；3.复制AC的snaddrs文件到Client。

# AC 的 IP 地址
AC_IP="10.0.4.10"

# Client 的 IP 地址
Client_IP="10.0.4.10"

# snNum 的数量（假设已经定义）
snNum=31

# 包含 IP 地址的文件
SN_ADDRS_FILE="/home/ubuntu/ECDS/data/snips"

# 要写入远端文件的字符串的前缀
prefix="sn"

# 密码
PASSWORD="jjp918JJP"

# 确保 snaddrs 文件存在
if [ ! -f "$SN_ADDRS_FILE" ]; then
    echo "snaddrs 文件不存在"
    exit 1
fi

# 清空当前的 snaddrs 文件（如果存在）
rm -f /home/ubuntu/ECDS/data/snaddrs

# 遍历 snaddrs 文件中的每一行 IP 地址
for (( i=1; i<=$snNum; i++ ))
do
    # 读取每个 IP 地址
    ip_addr=$(sed -n "${i}p" $SN_ADDRS_FILE)

    # 检查 IP 地址是否有效
    if [[ $ip_addr =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        ssh-keyscan -H $ip_addr >> ~/.ssh/known_hosts

        # 构建要写入的字符串
        data="${prefix}${i},${ip_addr}:50061"
        echo "将要写入的数据: $data"
        
        # 使用 sshpass 连接到每个 IP 地址并写入文件
        sshpass -p "$PASSWORD" ssh ubuntu@$ip_addr "echo '$data' > /home/ubuntu/ECDS/data/snaddrs"
        
        # 将数据追加到本地文件
        echo "$data" >> /home/ubuntu/ECDS/data/snaddrs
        
        # 检查 SSH 命令是否成功执行
        if [ $? -eq 0 ]; then
            echo "成功写入 $ip_addr 的 /home/ubuntu/ECDS/data/snaddrs 文件"
        else
            echo "写入 $ip_addr 的 /home/ubuntu/ECDS/data/snaddrs 文件失败"
        fi
    else
        echo "无效的 IP 地址: $ip_addr"
    fi
done

ssh-keyscan -H $Client_IP >> ~/.ssh/known_hosts

# 复制文件到Client
if sshpass -p "$PASSWORD" scp /home/ubuntu/ECDS/data/snaddrs ubuntu@$Client_IP:/home/ubuntu/ECDS/data; then
    echo "文件成功复制到 Client"
else
    echo "文件复制到 Client 失败"
fi

echo "所有数据已写入 AC 的 /home/ubuntu/ECDS/data/snaddrs 文件，并已复制到Client"
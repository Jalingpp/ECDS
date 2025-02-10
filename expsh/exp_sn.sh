if [ -z $1 ]; then
    dsnMode="ec"
else
    dsnMode=$1
fi

# 检查第二个参数是否为空
if [ -z "$2" ]; then
    snPort=50061
else
    snPort=$2
fi

# GOPath="/home/ubuntu/ECDS/expgo/"
GOPath="/root/DSN/ECDS/expgo/"
GOFile="exp_sn.go"
# datadir="/home/ubuntu/ECDS/data/snaddrfile/"
datadir="/root/DSN/ECDS/data/snaddrfile/"

# 将snPort添加到datadir后面
datadir="${datadir}${snPort}"

cd $GOPath

# nohup /usr/local/go/bin/go run $GOFile $dsnMode $datadir > "/home/ubuntu/ECDS/data/snlog/output_sn${snPort}.log" 2>&1 &
nohup /usr/local/go/bin/go run $GOFile $dsnMode $datadir > "/root/DSN/ECDS/data/snlog/output_sn${snPort}.log" 2>&1 &
sleep 1
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

GOPath="/home/ubuntu/ECDS/expgo/"
# GOPath="/root/DSN/ECDS/expgo/"
GOFile="exp_sn.go"
datadir="/home/ubuntu/ECDS/data/snaddrfile/"
# datadir="/root/DSN/ECDS/data/snaddrfile/"

# 将snPort添加到datadir后面
datadir="${datadir}${snPort}"

# 切换到GOPath目录
cd "$GOPath" || { echo "Failed to change directory to $GOPath"; exit 1; }

# 设置Go编译的临时文件目录为$GOPath下的temp目录
export TMPDIR="$GOPath/temp"
mkdir -p "$TMPDIR"

if [ "$snPort" -eq 50061 ]; then
    rm -rf "$TMPDIR"/*
    rm -rf ~/.cache/go-build/*
    /usr/local/go/bin/go build -o expsn $GOFile
fi

nohup ./expsn $dsnMode $datadir > "/home/ubuntu/ECDS/data/snlog/output_sn${snPort}.log" 2>&1 &
# nohup /usr/local/go/bin/go run $GOFile $dsnMode $datadir > "/home/ubuntu/ECDS/data/snlog/output_sn${snPort}.log" 2>&1 &
# nohup /usr/local/go/bin/go run $GOFile $dsnMode $datadir > "/root/DSN/ECDS/data/snlog/output_sn${snPort}.log" 2>&1 &
sleep 1
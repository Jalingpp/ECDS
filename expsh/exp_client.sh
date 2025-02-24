if [ -z $1 ]; then
    dsnMode="ec"
else
    dsnMode=$1
fi

if [ -z $2 ]; then
    clientnum=1
else
    clientnum=$2
fi

if [ -z $3 ]; then
    # datafiledir="/home/ubuntu/ECDS/data/NM/"
    datafiledir="/root/ECDS/data/NM/"
else
    datafiledir=$3
fi

if [ -z $4 ]; then
    datafilenum=1
else
    datafilenum=$4
fi

# GOPath="/home/ubuntu/ECDS/expgo/"
GOPath="/root/ECDS/expgo/"
GOFile="exp_client.go"
# datadir="/home/ubuntu/ECDS/data/"
datadir="/root/ECDS/data/"

# 切换到GOPath目录
cd "$GOPath" || { echo "Failed to change directory to $GOPath"; exit 1; }


# /usr/local/go/bin/go run $GOFile $dsnMode $datafiledir $clientnum $datafilenum $datadir

/usr/local/go/bin/go run $GOFile $dsnMode $datafiledir $clientnum $datafilenum $datadir > "/home/ubuntu/ECDS/data/clientlog/output_client.log" 2>&1

sleep 1
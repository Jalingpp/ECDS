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
    datafiledir="/home/ubuntu/ECDS/data/NM/"
else
    datafiledir=$3
fi

if [ -z $4 ]; then
    datafilenum=1
else
    datafilenum=$4
fi

GOPath="/home/ubuntu/ECDS/expgo/"
GOFile="exp_putfile_client.go"
datadir="/home/ubuntu/ECDS/data/"

cd $GOPath

/usr/local/go/bin/go run $GOFile $dsnMode $datafiledir $clientnum $datafilenum $datadir
sleep 1
#!/bin/bash

# 检查第一个参数是否为空
if [ -z "$1" ]; then
    SCRIPT_NAME="exp_ac.sh"
else
    SCRIPT_NAME=$1
fi

# 检查第一个参数是否为空
if [ -z "$2" ]; then
    dsnMode="ec"
else
    dsnMode=$2
fi

# 检查第一个参数是否为空
if [ -z "$3" ]; then
    clientnum=4
else
    clientnum=$3
fi

# 定义源脚本路径
# SCRIPT_PATH="/home/ubuntu/ECDS/expsh/"
SCRIPT_PATH="/root/ECDS/expsh/"
# datadir="/home/ubuntu/ECDS/data/"
datadir="/root/ECDS/data/"

nohup bash "$SCRIPT_PATH$SCRIPT_NAME" $dsnMode $clientnum > "$datadir/output_ac.log" 2>&1 &
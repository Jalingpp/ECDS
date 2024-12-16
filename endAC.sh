#!/bin/bash

# 检查第一个参数是否为空
if [ -z "$1" ]; then
    end_keyword="exp_putfile_ac"
else
    end_keyword=$1
fi

pkill -f $end_keyword
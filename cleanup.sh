#!/bin/bash
sleep 10  # 等待10秒，确保文件已生成并未被占用

# 删除文件
rm -f /root/csv_filter/script_mbs/input*.csv /root/csv_filter/script_mbs/output*.csv

# 检查命令的退出状态码
if [ $? -eq 0 ]; then
    echo "Files deleted successfully."
else
    echo "Failed to delete files."
fi

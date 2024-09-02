#!/bin/bash

# 等待10秒，确保文件已生成并未被占用
sleep 10

# 查看 input 和 output 文件的大小
echo "Checking file sizes and line counts before deletion:"
for file in /root/csv_filter/programs/input*.csv /root/csv_filter/programs/output*.csv; do
    if [ -f "$file" ]; then
        echo "File: $file"
        du -sh "$file"
        echo "Line count: $(wc -l < "$file")"
        echo "--------------------------"
    else
        echo "File not found: $file"
    fi
done

# 执行删除命令，并检查是否成功
rm -f /root/csv_filter/programs/input*.csv /root/csv_filter/programs/output*.csv

# 检查命令的退出状态码
if [ $? -eq 0 ]; then
    echo "Files deleted successfully."
else
    echo "Failed to delete files."
fi

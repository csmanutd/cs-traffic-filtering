#!/bin/bash

# 等待3秒，确保文件已生成并未被占用
sleep 3

cd /root/fsx/devtools/cs-traffic-filtering

# 查看 api 文件夹中所有 csv 文件的大小和行数
echo "检查删除前的文件大小和行数："
for file in api/*.csv; do
    if [ -f "$file" ]; then
        echo "文件: $file"
        du -sh "$file"
        echo "行数: $(wc -l < "$file")"
        echo "--------------------------"
    else
        echo "文件未找到: $file"
    fi
done

# 执行删除命令，并检查是否成功
rm -f api/*.csv

# 检查命令的退出状态码
if [ $? -eq 0 ]; then
    echo "文件删除成功。"
else
    echo "文件删除失败。"
fi

# 再次检查 api 文件夹，确认是否所有 csv 文件都已被删除
remaining_files=$(ls api/*.csv 2>/dev/null | wc -l)
if [ "$remaining_files" -eq 0 ]; then
    echo "api 文件夹中的所有 csv 文件已成功删除。"
else
    echo "警告：api 文件夹中仍有 $remaining_files 个 csv 文件。"
fi

#!/usr/bin/expect -f

# Change the directory as needed
cd /root/csv_filter/script_mbs

# Step 1: 执行程序api，并等待其完成
puts "Running program api..."
spawn ./api

# 等待程序api的提示并自动输入回车
expect "Enter the date (YYYYMMDD) to retrieve data:"
send "\r"

# 检查程序api是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program api failed to execute."
    exit 1
}
puts "Program api executed successfully."

# 检查是否生成了input.csv文件
if {![file exists "input.csv"]} {
    puts "input.csv file not found."
    exit 1
}
puts "input.csv file found."

# Step 2: 导入环境变量
set env(AWS_PROFILE) nris
puts "Environment variable AWS_PROFILE set to nris."

# Step 3: 执行程序ipl，并等待其完成
puts "Running program ipl..."
spawn ./ipl

# 等待程序ipl的提示并自动输入回车
expect "Please input the date of the file (YYYYMMDD)，Press Enter to use the default value (yesterday):"
send "\r"

# 检查程序ipl是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program ipl failed to execute."
    exit 1
}
puts "Program ipl executed successfully."

# 确保文件写入完成
sleep 2

# 检查是否生成了output开头的csv文件
set output_file [glob -nocomplain output*.csv]
if {[llength $output_file] == 0} {
    puts "No output*.csv file found."
    exit 1
}
puts "Output file $output_file found."

# Step 4: 删除以input和output开头的csv文件
sleep 10

puts "Current directory: [pwd]"
puts "Files to delete: [glob input*.csv output*.csv]"

# exec rm -f input*.csv output*.csv
# puts "All input*.csv and output*.csv files deleted."

exec /bin/bash /root/csv_filter/script_mbs/cleanup.sh >> /var/log/ipl.log

# 再次检查文件是否删除成功
#set remaining_files [glob input*.csv output*.csv]
#if {[llength $remaining_files] > 0} {
#    puts "Failed to delete some files: $remaining_files"
#} else {
#    puts "All input*.csv and output*.csv files deleted."
#}

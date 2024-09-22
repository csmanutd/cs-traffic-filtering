#!/usr/bin/expect -f

# 设置基础目录
set base_dir "/root/fsx/devtools/cs-traffic-filtering"
cd $base_dir

# Step 1: 执行程序api，并等待其完成
puts "Running program api..."
spawn sh -c "cd $base_dir/api && ./api"

# 等待程序api的提示并自动输入回车
expect "Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday):"
send "\r"

# 等待程序api执行完成的提示
expect "Data retrieval and CSV creation completed successfully. Output saved to *"
set csv_file $expect_out(0,string)
set csv_file [lindex [split $csv_file " "] end]
puts "CSV file created: $csv_file"

# 处理S3上传询问
expect "Do you want to upload the CSV file to S3? (Y/n):"
send "\r"

# 处理S3配置确认
expect "Do you want to use this configuration? (Y/n):"
send "\r"

# 等待上传完成
expect {
    "File successfully uploaded to S3" {
        puts "CSV file uploaded to S3 successfully."
    }
    timeout {
        puts "Timeout waiting for S3 upload confirmation."
    }
}

# 检查程序api是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program api failed to execute."
    exit 1
}
puts "Program api executed successfully."

# 获取昨天的日期
set yesterday [clock format [clock add [clock seconds] -1 days] -format "%Y%m%d"]

# 首先检查是否存在昨天的csv文件
set csv_file [glob -nocomplain api/${yesterday}.csv]
if {[llength $csv_file] == 0} {
    puts "Searching for yyyymmdd.csv file in api/ directory..."
    set csv_files [glob -nocomplain api/[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9].csv]
    if {[llength $csv_files] == 0} {
        puts "No yyyymmdd.csv files found in api/ directory."
        exit 1
    } else {
        # 按文件名排序，选择最新的文件
        set csv_file [lindex [lsort -decreasing $csv_files] 0]
        puts "Found CSV file: $csv_file"
    }
} else {
    puts "CSV file for yesterday ($csv_file) found."
}

# 提取文件名（不包含路径）
set csv_filename [file tail $csv_file]

# Step 2: 执行程序ipl，并等待其完成
puts "Running program ipl..."
spawn sh -c "cd $base_dir/ipl && ./ipl -input ../api/$csv_filename"

# 检查程序ipl是否成功执行
expect "CSV processing completed. Output saved to *"
expect "Do you want to upload the CSV file to S3? (Y/n):"
send "\r"

# 处理S3配置确认
expect "Do you want to use this configuration? (Y/n):"
send "\r"

# 等待上传完成
expect {
    "File successfully uploaded to S3" {
        puts "CSV file uploaded to S3 successfully."
    }
    timeout {
        puts "Timeout waiting for S3 upload confirmation."
    }
}

# 检查程序ipl是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program ipl failed to execute."
    exit 1
}
puts "Program ipl executed successfully."

# Step 3: 执行程序fl，并等待其完成
puts "Running program fl..."
spawn sh -c "cd $base_dir/fl && ./fl -input ../api/$csv_filename"

# 检查程序fl是否成功执行
expect "CSV filtering completed. Output saved to *"
expect "Do you want to upload the CSV file to S3? (Y/n):"
send "\r"

# 处理S3配置确认
expect "Do you want to use this configuration? (Y/n):"
send "\r"

# 等待上传完成
expect {
    "File successfully uploaded to S3" {
        puts "CSV file uploaded to S3 successfully."
    }
    timeout {
        puts "Timeout waiting for S3 upload confirmation."
    }
}

# 检查程序fl是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program fl failed to execute."
    exit 1
}
puts "Program fl executed successfully."

# Step 4: 执行程序gm，并等待其完成
puts "Running program gm..."
spawn sh -c "cd $base_dir/gm && ./gm -input ../api/$csv_filename"

# 检查程序gm是否成功执行
expect "CSV filtering completed. Output saved to *"
expect "Do you want to upload the CSV file to S3? (Y/n):"
send "\r"

# 处理S3配置确认
expect "Do you want to use this configuration? (Y/n):"
send "\r"

# 等待上传完成
expect {
    "File successfully uploaded to S3" {
        puts "CSV file uploaded to S3 successfully."
    }
    timeout {
        puts "Timeout waiting for S3 upload confirmation."
    }
}

# 检查程序gm是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program gm failed to execute."
    exit 1
}
puts "Program gm executed successfully."

# Step 5: 执行cleanup.sh
exec /bin/bash $base_dir/cleanup.sh >> /var/log/ipl.log
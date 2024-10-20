#!/usr/bin/expect -f

# 设置更长的超时时间（例如10分钟）
set timeout 600

# 设置基础目录
set base_dir "/root/fsx/devtools/filtering"
cd $base_dir

# 步骤1：执行api程序并等待完成
puts "Running program api..."
spawn sh -c "cd $base_dir/api && ./api"

# 等待api程序提示并自动输入
expect "Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday):"
send "\r"

# 等待api程序完成提示
expect {
    "Data retrieval and CSV creation completed successfully. Output saved to *" {
        set csv_file $expect_out(0,string)
        set csv_file [lindex [split $csv_file " "] end]
        puts "CSV file created: $csv_file"
    }
    timeout {
        puts "Timeout waiting for CSV creation."
        exit 1
    }
}

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
        exit 1
    }
}

# 检查api程序是否成功执行
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program api failed to execute."
    exit 1
}
puts "Program api executed successfully."
#!/usr/bin/expect -f

# 设置超时时间为30分钟
set timeout 1800

# 设置基础目录
set base_dir "/root/fsx/devtools/filtering"
cd $base_dir

# 定义最大重试次数和重试间隔（秒）
set max_retries 5
set retry_interval 60

# 定义执行api的过程
proc run_api {} {
    global base_dir
    spawn sh -c "cd $base_dir/api && ./api"

    expect {
        "Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday): " {
            send "\r"
        }
        timeout {
            puts "Timeout waiting for input prompt."
            return 0
        }
    }

    # 检查API响应
    expect {
        -re "Data retrieval and CSV creation completed successfully\\. Output saved to (\\S+\\.csv)" {
            set ::csv_file $expect_out(1,string)
            puts "CSV file created: $::csv_file"
            return 1
        }
        -re ".*5\[0-9\]\[0-9\].*" {
            # 匹配任何5xx错误（500, 501, 502, 503, 504等）
            puts "Encountered 5xx server error. Will retry..."
            return 0
        }
        timeout {
            puts "Timeout waiting for CSV creation."
            return 0
        }
    }
}

# 使用重试逻辑执行api
set retry_count 0
set success 0

while {$retry_count < $max_retries && !$success} {
    if {$retry_count > 0} {
        puts "Retry attempt $retry_count of $max_retries after waiting $retry_interval seconds..."
        exec sleep $retry_interval
    }
    
    set success [run_api]
    incr retry_count
}

if {!$success} {
    puts "Failed to execute api program after $max_retries attempts."
    exit 1
}

# 处理S3上传询问
expect "Do you want to upload the CSV file to S3? (Y/n): "
send "Y\r"

# 处理S3配置确认
expect "Do you want to use this configuration? (Y/n): "
send "Y\r"

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

# 移动CSV文件到filter_cli文件夹
puts "Moving CSV file to filter_cli folder..."
exec mv "$base_dir/api/$csv_file" "$base_dir/filter_cli/"

# 定义要执行的预设列表
set presets {fL gM NPOQ}

# 循环执行每个预设
foreach preset $presets {
    puts "Running filter_cli with preset $preset..."
    spawn sh -c "cd $base_dir/filter_cli && ./filter_cli -input $csv_file -preset $preset"
    
    # 增加超时时间检查
    expect {
        "File successfully uploaded to S3 bucket" {
            puts "Filter_cli completed successfully for preset $preset"
        }
        timeout {
            puts "Timeout waiting for filter_cli completion with preset $preset after 30 minutes"
            exit 1
        }
        eof {
            puts "Filter_cli ended unexpectedly for preset $preset"
            exit 1
        }
    }
}

puts "All operations completed successfully."

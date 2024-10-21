#!/usr/bin/expect -f

# 设置超时时间为15分钟
set timeout 900

# 设置基础目录
set base_dir "/root/fsx/devtools/filtering"
cd $base_dir

# 步骤1：执行api程序并等待完成
puts "Running program api..."
spawn sh -c "cd $base_dir/api && ./api"

# 等待api程序提示并自动输入
expect "Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday): "
send "\r"

# 等待api程序完成提示
expect {
    -re "Data retrieval and CSV creation completed successfully\\. Output saved to (\\S+\\.csv)" {
        set csv_file $expect_out(1,string)
        puts "CSV file created: $csv_file"
    }
    timeout {
        puts "Timeout waiting for CSV creation."
        exit 1
    }
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
    
    expect {
        "File successfully uploaded to S3 bucket" {
            puts "Filter_cli completed successfully for preset $preset"
        }
        timeout {
            puts "Timeout waiting for filter_cli completion with preset $preset"
            exit 1
        }
        eof {
            puts "Filter_cli ended unexpectedly for preset $preset"
            exit 1
        }
    }
}

puts "All operations completed successfully."

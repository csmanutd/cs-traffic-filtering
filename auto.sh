#!/usr/bin/expect -f

# 设置超时时间为30分钟
set timeout 1800

# 设置基础目录
set base_dir "/root/fsx/devtools/filtering"
cd $base_dir

# Execute cleanup script first
puts "Executing cleanup script..."
catch {exec sh $base_dir/cleanup.sh} cleanup_result
puts $cleanup_result

# 定义最大重试次数
set max_retries 5
set retry_count 0
set retry_wait 60

# 执行api_auto程序
puts "Running api_auto..."

while {$retry_count < $max_retries} {
    spawn sh -c "cd $base_dir/api_auto && ./api_auto"
    
    expect {
        -re "Error during data retrieval: request failed with status code: 50\[0-9\]" {
            incr retry_count
            puts "Encountered 5xx error. Retry attempt $retry_count of $max_retries"
            puts "Waiting for $retry_wait seconds before retrying..."
            exec sleep $retry_wait
            continue
        }
        -re "Data retrieval, CSV creation and S3 upload completed successfully\\. Output saved to (\\S+\\.csv)" {
            set csv_file $expect_out(1,string)
            puts "CSV file created and uploaded: $csv_file"
            break
        }
        timeout {
            puts "Timeout waiting for api_auto response."
            exit 1
        }
        eof {
            puts "api_auto ended unexpectedly."
            exit 1
        }
    }
}

if {$retry_count >= $max_retries} {
    puts "Maximum retry attempts reached. Exiting."
    exit 1
}

# 移动CSV文件到filter_cli文件夹
puts "Moving CSV file to filter_cli folder..."
exec mv "$base_dir/api_auto/$csv_file" "$base_dir/filter_cli/"

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

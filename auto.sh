#!/usr/bin/expect -f

# Set base directory
set base_dir "/root/fsx/devtools/filtering"
cd $base_dir

# Step 1: Execute api program and wait for completion
puts "Running program api..."
spawn sh -c "cd $base_dir/api && ./api"

# Wait for api program prompt and automatically enter
expect "Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday):"
send "\r"

# Wait for api program completion prompt
expect "Data retrieval and CSV creation completed successfully. Output saved to *"
set csv_file $expect_out(0,string)
set csv_file [lindex [split $csv_file " "] end]
puts "CSV file created: $csv_file"

# Handle S3 upload inquiry
expect "Do you want to upload the CSV file to S3? (Y/n):"
send "\r"

# Handle S3 configuration confirmation
expect "Do you want to use this configuration? (Y/n):"
send "\r"

# Wait for upload completion
expect {
    "File successfully uploaded to S3" {
        puts "CSV file uploaded to S3 successfully."
    }
    timeout {
        puts "Timeout waiting for S3 upload confirmation."
    }
}

# Check if api program executed successfully
expect eof
set exit_status [wait]
if {[lindex $exit_status 3] != 0} {
    puts "Program api failed to execute."
    exit 1
}
puts "Program api executed successfully."
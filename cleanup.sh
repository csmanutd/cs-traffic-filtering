#!/bin/bash

# Wait for 3 seconds to ensure files are generated and not in use
sleep 3

cd /root/fsx/devtools/filtering

# Check if any CSV files exist in filter_cli folder
csv_files=$(ls filter_cli/*.csv 2>/dev/null)

if [ -n "$csv_files" ]; then
    # Display size and line count of CSV files before deletion
    echo "Checking file sizes and line counts before deletion:"
    for file in $csv_files; do
        echo "File: $file"
        du -sh "$file"
        echo "Lines: $(wc -l < "$file")"
        echo "--------------------------"
    fi

    # Execute deletion command
    rm -f filter_cli/*.csv
    
    if [ $? -eq 0 ]; then
        echo "Files deleted successfully."
    else
        echo "Failed to delete files."
    fi
else
    echo "No CSV files found in filter_cli folder."
fi

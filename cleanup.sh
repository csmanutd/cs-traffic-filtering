#!/bin/bash

sleep 3

cd /root/fsx/devtools/filtering

csv_files=$(ls filter_cli/*.csv 2>/dev/null)

if [ -n "$csv_files" ]
then
    echo "Checking file sizes and line counts before deletion:"
    for file in $csv_files
    do
        echo "File: $file"
        du -sh "$file"
        echo "Lines: $(wc -l < "$file")"
        echo "--------------------------"
    done
    
    rm -f filter_cli/*.csv
    echo "Files deleted successfully."
else
    echo "No CSV files found in filter_cli folder."
fi

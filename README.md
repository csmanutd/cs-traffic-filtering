# cs-traffic-filtering
Get traffic data from CloudSecure and filter the data by iplists.

1. Run the api script.    
   For the 1st time, it will ask you to specify the API key, API secret and tenant id.  
   Then save this information to a JSON file cloudsecure.config  
   Next time, it will load the configuration from the JSON file.  
    
   This script will retrieve all traffic logs from CloudSecure and export to a csv file(input.csv).  
   You can specify the date to retrieve. By default (just press enter), it will retrieve yesterday's traffic logs.  
   
2. Run the ipl script.  
   First, you must create a text file subnets.txt and put all the IP CIDR blocks in it.  
   The script will compare the src and dst IP addresses with the CIDR blocks.  
   If any addresses don't belong to all of the CIDR blocks, the traffic log will be output to a csv file.   
   The script will also filter out all DENIED traffic.   
  
   Lastly, it will upload the input and output csv files to the s3 bucket.  
   Please change the bucket name and folder as needed. Make sure you can access the s3 bucket with AWS CLI in your environment.    
   
   By default, the script will append the date to the filename.  
   Then check the s3 bucket to see if there is a file with the same filename.  
   If there is, it will append '_x' (x means numbers) and then upload.  

3. If you need full automation. Use the autoscript.sh with crontab or systemctl.
   But you have to change the directory to your work directory.   

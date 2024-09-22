package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/csmanutd/s3utils" // Import the s3utils package

	"github.com/csmanutd/csutils"
)

// LoadConfig 从JSON文件读取配置
func LoadConfig(fileName string) (csutils.CloudSecureConfig, error) {
	return csutils.LoadOrCreateCloudSecureConfig(fileName)
}

// SaveConfig 将配置保存到JSON文件
func SaveConfig(fileName string, config csutils.CloudSecureConfig) error {
	return csutils.SaveCloudSecureConfig(fileName, config)
}

// PromptUserInput 提示用户输入API凭证和租户ID
func PromptUserInput() csutils.CloudSecureInfo {
	return csutils.CreateNewCloudSecureInfo()
}

func createFlowReport(apiKey, apiSecret, tenantID, fileName, fileFormat, fromTime, toTime string, maxResults int) ([]map[string]interface{}, error) {
	url := "https://cloud.illum.io/api/v1/flows"

	// Encode the API key and secret
	credentials := fmt.Sprintf("%s:%s", apiKey, apiSecret)
	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(credentials))

	headers := map[string]string{
		"accept":        "*/*",
		"content-type":  "application/json",
		"Authorization": "Basic " + encodedCredentials,
		"x-tenant-id":   tenantID,
	}

	data := map[string]interface{}{
		"fileName":   fileName,
		"fileFormat": "FILE_FORMAT_" + strings.ToUpper(fileFormat),
		"period": map[string]string{
			"start_time": fromTime,
			"end_time":   toTime,
		},
		"max_results": maxResults,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshaling data: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body) // 使用io包的ReadAll函数
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	// Parse the response as JSON
	var jsonResponse map[string]interface{}
	err = json.Unmarshal(body, &jsonResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	// Extract the "flows" part of the response
	flows, ok := jsonResponse["flows"].([]interface{})
	if !ok || len(flows) == 0 {
		return nil, fmt.Errorf("no flows data found in the response")
	}

	// Convert flows to a slice of maps
	result := make([]map[string]interface{}, len(flows))
	for i, flow := range flows {
		result[i] = flow.(map[string]interface{})
	}

	return result, nil
}

func writeCSV(fileName string, data []map[string]interface{}, appendMode bool) error {
	// Fixed header order and new names
	headersList := []string{"FlowStatus", "FirstDetected", "LastDetected", "Source_IP", "Destination_IP", "DestinationPort", "Protocol", "ByteCount"}
	originalHeaders := []string{"status", "start_time", "end_time", "src", "dst", "dst_port", "protocol", "bytes"}

	// Open the CSV file with append mode if necessary
	fileMode := os.O_CREATE | os.O_WRONLY
	if appendMode {
		fileMode |= os.O_APPEND
	}

	file, err := os.OpenFile(fileName, fileMode, 0644)
	if err != nil {
		return fmt.Errorf("error creating/opening file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write the header to the CSV file only if not in append mode
	if !appendMode {
		if err := writer.Write(headersList); err != nil {
			return fmt.Errorf("error writing CSV header: %v", err)
		}
	}

	// Regular expression to extract the IP address
	re := regexp.MustCompile(`ip_address:([\d\.]+)`)

	// Write the values to the CSV file
	for _, flowMap := range data {
		record := make([]string, len(headersList))
		for i, originalHeader := range originalHeaders {
			value := flowMap[originalHeader]
			valueStr := fmt.Sprintf("%v", value)

			// Clean up Source_IP and Destination_IP columns
			if originalHeader == "src" || originalHeader == "dst" {
				matches := re.FindStringSubmatch(valueStr)
				if len(matches) > 1 {
					valueStr = matches[1]
				} else {
					valueStr = ""
				}
			}

			record[i] = valueStr
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("error writing CSV record: %v", err)
		}
	}

	return nil
}

// S3Config represents the S3 configuration
type S3Config struct {
	BucketName  string `json:"bucket_name"`
	FolderName  string `json:"folder_name"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"` // 新增
}

// LoadS3Config loads S3 configuration from a JSON file
func LoadS3Config(fileName string) (S3Config, error) {
	var config S3Config
	file, err := os.ReadFile(fileName)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(file, &config)
	return config, err
}

// SaveS3Config saves S3 configuration to a JSON file
func SaveS3Config(fileName string, config S3Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0644)
}

func main() {
	const configFileName = "csconfig.json"

	// Add command-line flags
	csName := flag.String("cs", "", "Specify CloudSecure name")
	outputFile := flag.String("out", "", "Specify output CSV file name")
	flag.Parse()

	// Load configuration
	config, err := LoadConfig(configFileName)
	if err != nil {
		fmt.Println("Config file not found. Please enter your API credentials.")
		config.CloudSecures = make(map[string]csutils.CloudSecureInfo)
		config.DefaultCloudName = addNewCloudSecure(&config)
		SaveConfig(configFileName, config)
		fmt.Println("Config file saved.")
	}

	// Determine which CloudSecure to use
	selectedCS := config.DefaultCloudName
	if *csName != "" {
		selectedCS = *csName
	}

	// Check if the specified CloudSecure exists
	for {
		if _, exists := config.CloudSecures[selectedCS]; !exists {
			fmt.Printf("CloudSecure '%s' not found. Add a new tenant? (Y/n): ", selectedCS)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "" || response == "y" {
				selectedCS = addNewCloudSecure(&config)
				SaveConfig(configFileName, config)
			} else {
				fmt.Print("Enter CloudSecure name: ")
				selectedCS, _ = reader.ReadString('\n')
				selectedCS = strings.TrimSpace(selectedCS)
			}
		} else {
			break
		}
	}

	fmt.Printf("Using CloudSecure: %s\n", selectedCS)

	// Prompt user for date input
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday): ")
	dateInput, _ := reader.ReadString('\n')
	dateInput = strings.TrimSpace(dateInput)

	// If no date is provided, use the previous day
	var date time.Time
	if dateInput == "" {
		date = time.Now().AddDate(0, 0, -1)
		dateInput = date.Format("20060102")
	} else {
		date, err = time.Parse("20060102", dateInput)
		if err != nil {
			fmt.Printf("Invalid date format: %v\n", err)
			os.Exit(1)
		}
	}

	// Set default output file name if not specified
	if *outputFile == "" {
		*outputFile = dateInput + ".csv"
	}

	// Define time segments (6 segments, 4 hours each, reverse order)
	timeSegments := []struct {
		fromTime string
		toTime   string
	}{
		{fromTime: date.Add(20 * time.Hour).Format(time.RFC3339), toTime: date.Add(24 * time.Hour).Format(time.RFC3339)},
		{fromTime: date.Add(16 * time.Hour).Format(time.RFC3339), toTime: date.Add(20 * time.Hour).Format(time.RFC3339)},
		{fromTime: date.Add(12 * time.Hour).Format(time.RFC3339), toTime: date.Add(16 * time.Hour).Format(time.RFC3339)},
		{fromTime: date.Add(8 * time.Hour).Format(time.RFC3339), toTime: date.Add(12 * time.Hour).Format(time.RFC3339)},
		{fromTime: date.Add(4 * time.Hour).Format(time.RFC3339), toTime: date.Add(8 * time.Hour).Format(time.RFC3339)},
		{fromTime: date.Format(time.RFC3339), toTime: date.Add(4 * time.Hour).Format(time.RFC3339)},
	}

	// Loop through each time segment, retrieve data, and write to CSV immediately
	for i, segment := range timeSegments {
		data, err := createFlowReport(config.CloudSecures[selectedCS].APIKey, config.CloudSecures[selectedCS].APISecret, config.CloudSecures[selectedCS].TenantID, *outputFile, "csv", segment.fromTime, segment.toTime, 10000000)
		if err != nil {
			fmt.Printf("Error during data retrieval: %v\n", err)
			os.Exit(1)
		}

		appendMode := i > 0 // Only append for the second segment onwards
		err = writeCSV(*outputFile, data, appendMode)
		if err != nil {
			fmt.Printf("Error writing to CSV: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Data retrieval and CSV creation completed successfully. Output saved to %s\n", *outputFile)

	// Ask user if they want to upload to S3
	fmt.Print("Do you want to upload the CSV file to S3? (Y/n): ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "y" {
		s3Config, err := LoadS3Config("s3config.json")
		configChanged := false
		if err == nil {
			fmt.Printf("Current S3 configuration:\nBucket: %s\nFolder: %s\nProfile: %s\n",
				s3Config.BucketName, s3Config.FolderName, s3Config.ProfileName)
			fmt.Print("Do you want to use this configuration? (Y/n): ")
			useExisting, _ := reader.ReadString('\n')
			useExisting = strings.TrimSpace(strings.ToLower(useExisting))

			if useExisting != "" && useExisting != "y" {
				s3Config = S3Config{} // Reset configuration
				configChanged = true
			}
		} else {
			s3Config = S3Config{} // Create new configuration if loading fails
			configChanged = true
		}

		if s3Config == (S3Config{}) {
			fmt.Print("Enter S3 bucket name: ")
			s3Config.BucketName, _ = reader.ReadString('\n')
			s3Config.BucketName = strings.TrimSpace(s3Config.BucketName)

			fmt.Print("Enter S3 folder name: ")
			s3Config.FolderName, _ = reader.ReadString('\n')
			s3Config.FolderName = strings.TrimSpace(s3Config.FolderName)

			fmt.Print("Enter AWS profile name: ")
			s3Config.ProfileName, _ = reader.ReadString('\n')
			s3Config.ProfileName = strings.TrimSpace(s3Config.ProfileName)
			configChanged = true
		}

		// Upload file to S3
		err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, *outputFile, s3Config.BucketName, s3Config.FolderName)
		if err != nil {
			fmt.Printf("Error uploading file to S3: %v\n", err)
		} else {
			fmt.Println("File successfully uploaded to S3")
			// Only save if configuration has changed
			if configChanged {
				err = SaveS3Config("s3config.json", s3Config)
				if err != nil {
					fmt.Printf("Error saving S3 configuration: %v\n", err)
				} else {
					fmt.Println("S3 configuration saved")
				}
			}
		}
	}
}

func addNewCloudSecure(config *csutils.CloudSecureConfig) string {
	cloudSecureInfo := PromptUserInput()

	fmt.Print("Enter CloudSecure Name: ")
	reader := bufio.NewReader(os.Stdin)
	cloudSecureName, _ := reader.ReadString('\n')
	cloudSecureName = strings.TrimSpace(cloudSecureName)

	config.CloudSecures[cloudSecureName] = cloudSecureInfo
	return cloudSecureName
}

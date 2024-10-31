package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
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

	// 加载配置
	config, err := LoadConfig(configFileName)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 使用默认的CloudSecure
	selectedCS := config.DefaultCloudName
	if _, exists := config.CloudSecures[selectedCS]; !exists {
		fmt.Printf("Default CloudSecure '%s' not found in config\n", selectedCS)
		os.Exit(1)
	}

	// 使用前一天的日期
	date := time.Now().AddDate(0, 0, -1)
	dateStr := date.Format("20060102")
	outputFile := dateStr + ".csv"

	// 定义时间段（6个时间段，每段4小时，倒序）
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

	// 循环处理每个时间段
	for i, segment := range timeSegments {
		data, err := createFlowReport(
			config.CloudSecures[selectedCS].APIKey,
			config.CloudSecures[selectedCS].APISecret,
			config.CloudSecures[selectedCS].TenantID,
			outputFile,
			"csv",
			segment.fromTime,
			segment.toTime,
			10000000,
		)
		if err != nil {
			fmt.Printf("Error during data retrieval: %v\n", err)
			os.Exit(1)
		}

		appendMode := i > 0
		err = writeCSV(outputFile, data, appendMode)
		if err != nil {
			fmt.Printf("Error writing to CSV: %v\n", err)
			os.Exit(1)
		}
	}

	// 自动上传到S3
	s3Config, err := LoadS3Config("s3config.json")
	if err != nil {
		fmt.Printf("Error loading S3 config: %v\n", err)
		os.Exit(1)
	}

	err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, outputFile, s3Config.BucketName, s3Config.FolderName)
	if err != nil {
		fmt.Printf("Error uploading to S3: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Data retrieval, CSV creation and S3 upload completed successfully. Output saved to %s\n", outputFile)
}

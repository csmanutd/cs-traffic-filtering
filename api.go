package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"bufio"
	"strings"
	"time"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"net/http"
	"regexp"
)

// Config struct holds the API credentials and tenant ID
type Config struct {
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
	TenantID  string `json:"tenantID"`
}

// LoadConfig reads the configuration from a JSON file
func LoadConfig(fileName string) (Config, error) {
	var config Config

	// Check if the file exists
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return config, fmt.Errorf("config file not found")
	}

	// Read the file
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return config, err
	}

	// Parse the JSON data
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

// SaveConfig saves the configuration to a JSON file
func SaveConfig(fileName string, config Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fileName, data, 0644)
}

// PromptUserInput prompts the user to input the API credentials and tenant ID
func PromptUserInput() Config {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Please enter your API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Please enter your API Secret: ")
	apiSecret, _ := reader.ReadString('\n')
	apiSecret = strings.TrimSpace(apiSecret)

	fmt.Print("Please enter your Tenant ID: ")
	tenantID, _ := reader.ReadString('\n')
	tenantID = strings.TrimSpace(tenantID)

	return Config{
		APIKey:    apiKey,
		APISecret: apiSecret,
		TenantID:  tenantID,
	}
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

	body, err := ioutil.ReadAll(resp.Body)
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

func main() {
	const configFileName = "cloudsecure.config"

	// Load or prompt for configuration
	config, err := LoadConfig(configFileName)
	if err != nil {
		fmt.Println("Config file not found. Please enter your API credentials.")
		config = PromptUserInput()

		// Save the configuration
		err := SaveConfig(configFileName, config)
		if err != nil {
			fmt.Printf("Failed to save config file: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Config file saved.")
	} else {
		fmt.Println("Config file loaded successfully.")
	}

	// Prompt user for date input
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the date (YYYYMMDD) to retrieve data (leave empty for yesterday): ")
	dateInput, _ := reader.ReadString('\n')
	dateInput = strings.TrimSpace(dateInput)

	// If no date is provided, use the previous day
	var date time.Time
	if dateInput == "" {
		date = time.Now().AddDate(0, 0, -1)
	} else {
		date, err = time.Parse("20060102", dateInput)
		if err != nil {
			fmt.Printf("Invalid date format: %v\n", err)
			os.Exit(1)
		}
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
		data, err := createFlowReport(config.APIKey, config.APISecret, config.TenantID, "input.csv", "csv", segment.fromTime, segment.toTime, 10000000)
		if err != nil {
			fmt.Printf("Error during data retrieval: %v\n", err)
			os.Exit(1)
		}

		appendMode := i > 0  // Only append for the second segment onwards
		err = writeCSV("input.csv", data, appendMode)
		if err != nil {
			fmt.Printf("Error writing to CSV: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("Data retrieval and CSV creation completed successfully.")
}

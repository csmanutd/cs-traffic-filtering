package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/csmanutd/s3utils" // Import the s3utils package
)

// Check if an IP is in a list of IPs
func isIPInList(ip string, ipList map[string]bool) bool {
	return ipList[ip]
}

// Check if an IP is a public IP
func isPublicIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	// Check if IP is in private, link-local, loopback, multicast, or limited broadcast ranges
	privateIPBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
		"224.0.0.0/4",
		"255.255.255.255/32",
	}
	for _, block := range privateIPBlocks {
		_, cidr, _ := net.ParseCIDR(block)
		if cidr.Contains(parsedIP) {
			return false
		}
	}
	return true
}

// Load IPs from a file into a map
func loadIPs(filename string) (map[string]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ipMap := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		ipMap[ip] = true
	}
	return ipMap, scanner.Err()
}

func main() {
	inputFile := flag.String("input", "", "Input CSV file to be filtered")
	flag.Parse()

	if *inputFile == "" {
		fmt.Println("Please provide an input file using the -input flag")
		return
	}

	// Load IP lists
	fIPs, err := loadIPs("f.txt")
	if err != nil {
		fmt.Printf("Error loading f.txt: %v\n", err)
		return
	}
	lIPs, err := loadIPs("l.txt")
	if err != nil {
		fmt.Printf("Error loading l.txt: %v\n", err)
		return
	}
	knownIPs, err := loadIPs("known.txt")
	if err != nil {
		fmt.Printf("Error loading known.txt: %v\n", err)
		return
	}

	// Open input CSV file
	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Printf("Error opening input file: %v\n", err)
		return
	}
	defer file.Close()

	outputFile := strings.TrimSuffix(*inputFile, ".csv") + "_fl_filtered.csv"
	writer, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	defer writer.Close()

	reader := csv.NewReader(file)
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	// Read and process CSV file
	header, err := reader.Read()
	if err != nil {
		fmt.Printf("Error reading CSV header: %v\n", err)
		return
	}
	csvWriter.Write(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			continue
		}

		if len(record) < 8 {
			fmt.Printf("Skipping record with insufficient fields: %v\n", record)
			continue
		}

		flowStatus := record[0]
		sourceIP := record[3]
		destIP := record[4]

		if flowStatus == "ALLOWED" && (isIPInList(sourceIP, fIPs) || isIPInList(sourceIP, lIPs)) && !isPublicIP(destIP) && !isIPInList(destIP, knownIPs) {
			csvWriter.Write(record)
		}
	}

	fmt.Printf("CSV filtering completed. Output saved to %s\n", outputFile)

	// Ask user if they want to upload to S3
	inputReader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you want to upload the CSV file to S3? (Y/n): ")
	response, _ := inputReader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "y" {
		s3Config, err := LoadS3Config("s3config.json")
		configChanged := false
		if err == nil {
			fmt.Printf("Current S3 configuration:\nBucket: %s\nFolder: %s\nProfile: %s\n",
				s3Config.BucketName, s3Config.FolderName, s3Config.ProfileName)
			fmt.Print("Do you want to use this configuration? (Y/n): ")
			useExisting, _ := inputReader.ReadString('\n')
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
			s3Config.BucketName, _ = inputReader.ReadString('\n')
			s3Config.BucketName = strings.TrimSpace(s3Config.BucketName)

			fmt.Print("Enter S3 folder name: ")
			s3Config.FolderName, _ = inputReader.ReadString('\n')
			s3Config.FolderName = strings.TrimSpace(s3Config.FolderName)

			fmt.Print("Enter AWS profile name: ")
			s3Config.ProfileName, _ = inputReader.ReadString('\n')
			s3Config.ProfileName = strings.TrimSpace(s3Config.ProfileName)
			configChanged = true
		}

		// Upload file to S3
		err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, outputFile, s3Config.BucketName, s3Config.FolderName)
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

// S3Config represents the S3 configuration
type S3Config struct {
	BucketName  string `json:"bucket_name"`
	FolderName  string `json:"folder_name"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
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

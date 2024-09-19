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
	"path/filepath"
	"strings"

	"github.com/csmanutd/s3utils" // Import the s3utils package
)

// Read subnets from file
func readSubnets(filename string) ([]*net.IPNet, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var subnets []*net.IPNet
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		_, subnet, err := net.ParseCIDR(scanner.Text())
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, subnet)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return subnets, nil
}

// Judge if IP is in subnet
func isIPInSubnets(ip string, subnets []*net.IPNet) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, subnet := range subnets {
		if subnet.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// Extract rows that do not meet the criteria and save to a new CSV file, keeping the header
func extractIPsFromCSV(inputFile, outputFile string, subnets []*net.IPNet) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	output, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer output.Close()

	writer := csv.NewWriter(output)
	defer writer.Flush()

	header, err := reader.Read()
	if err != nil {
		return err
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Log the error and continue with the next record
			fmt.Printf("Error reading record: %v\n", err)
			continue
		}

		// Skip records with incorrect number of fields
		if len(record) < 2 {
			fmt.Printf("Skipping malformed record: %v\n", record)
			continue
		}

		// Only process rows where the first column starts with "ALLOWED"
		if strings.HasPrefix(record[0], "ALLOWED") {
			extract := false
			for _, field := range record[1:] { // Assume IP addresses start from the second column
				if field != "" && net.ParseIP(field) != nil && !isIPInSubnets(field, subnets) {
					extract = true
					break
				}
			}

			if extract {
				if err := writer.Write(record); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func main() {
	// Define CLI flags
	inputFile := flag.String("input", "", "Input CSV file name")
	outputFile := flag.String("output", "", "Output CSV file name (optional)")
	flag.Parse()

	// Check if input file is provided
	if *inputFile == "" {
		fmt.Println("Error: Input file is required. Use -input flag to specify the input file.")
		return
	}

	// Generate output file name if not provided
	if *outputFile == "" {
		ext := filepath.Ext(*inputFile)
		baseName := strings.TrimSuffix(*inputFile, ext)
		*outputFile = baseName + "_ipl_filtered" + ext
	}

	// Read subnets
	subnets, err := readSubnets("subnets.txt")
	if err != nil {
		fmt.Println("Error reading subnets:", err)
		return
	}

	// Process CSV
	err = extractIPsFromCSV(*inputFile, *outputFile, subnets)
	if err != nil {
		fmt.Println("Error occurred during CSV processing:", err)
		return
	}

	fmt.Printf("CSV processing completed. Output saved to %s\n", *outputFile)

	// Ask user if they want to upload to S3
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you want to upload the CSV file to S3? (Y/n): ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "y" {
		s3Config, err := LoadS3Config("s3config.json")
		if err == nil {
			fmt.Printf("Current S3 configuration:\nBucket: %s\nFolder: %s\nProfile: %s\n",
				s3Config.BucketName, s3Config.FolderName, s3Config.ProfileName)
			fmt.Print("Do you want to use this configuration? (Y/n): ")
			useExisting, _ := reader.ReadString('\n')
			useExisting = strings.TrimSpace(strings.ToLower(useExisting))

			if useExisting != "" && useExisting != "y" {
				s3Config = S3Config{} // Reset config if user doesn't want to use existing
			}
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
		}

		// Upload file to S3
		err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, *outputFile, s3Config.BucketName, s3Config.FolderName)
		if err != nil {
			fmt.Printf("Error uploading file to S3: %v\n", err)
		} else {
			fmt.Println("File successfully uploaded to S3")
			// Save S3 configuration
			err = SaveS3Config("s3config.json", s3Config)
			if err != nil {
				fmt.Printf("Error saving S3 configuration: %v\n", err)
			} else {
				fmt.Println("S3 configuration saved")
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

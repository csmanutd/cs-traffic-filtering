package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// 读取subnet信息的函数
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

// 判断IP是否在subnet中的函数
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

// 从CSV中提取不符合条件的行并保存到新的CSV文件，保留header
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
		// Depending on the requirements, choose to exit here or continue
		// return
	}

	fmt.Printf("CSV processing completed. Output saved to %s\n", *outputFile)
}

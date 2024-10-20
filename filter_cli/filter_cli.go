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

	"github.com/csmanutd/s3utils"
)

// FilterCondition 定义过滤条件
type FilterCondition struct {
	Field     string
	Operator  string
	ListFiles []string
}

// S3Config 表示S3配置
type S3Config struct {
	PresetName  string `json:"preset_name"`
	BucketName  string `json:"bucket_name"`
	FolderName  string `json:"folder_name"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
}

// Preset 表示保存的过滤器配置
type Preset struct {
	Name       string            `json:"name"`
	Conditions []FilterCondition `json:"conditions"`
	FlowStatus string            `json:"flow_status"`
}

// 加载IP函数
func loadIPs(filename string) ([]net.IPNet, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %v", err)
	}
	filename = absPath

	fmt.Println("Attempting to load IPs from file:", filename)

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening IP list file: %v", err)
	}
	defer file.Close()

	var ipNets []net.IPNet
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ipOrCIDR := strings.TrimSpace(scanner.Text())
		_, ipNet, err := net.ParseCIDR(ipOrCIDR)
		if err != nil {
			// If not a CIDR, try as a single IP
			ip := net.ParseIP(ipOrCIDR)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP or CIDR: %s", ipOrCIDR)
			}
			ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		}
		ipNets = append(ipNets, *ipNet)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading IP list file: %v", err)
	}

	fmt.Printf("Successfully loaded %d IPs from file\n", len(ipNets))
	return ipNets, nil
}

// 检查IP是否在列表中的函数
func isIPInList(ip string, ipNets []net.IPNet) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, ipNet := range ipNets {
		if ipNet.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// 检查是否为公共IP的函数
func isPublicIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	privateIPBlocks := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
		"127.0.0.0/8", "224.0.0.0/4", "255.255.255.255/32",
	}
	for _, block := range privateIPBlocks {
		_, cidr, _ := net.ParseCIDR(block)
		if cidr.Contains(parsedIP) {
			return false
		}
	}
	return true
}

// 过滤CSV文件的函数
func filterCSV(inputFile, outputFile string, conditions []FilterCondition, flowStatus string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("error opening input file: %v", err)
	}
	defer file.Close()

	// Create output file
	writer, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer writer.Close()

	reader := csv.NewReader(file)
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	// Read and write header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("error reading CSV header: %v", err)
	}
	csvWriter.Write(header)

	// Load IP lists
	ipLists := make(map[string][]net.IPNet)
	for _, cond := range conditions {
		for _, listFile := range cond.ListFiles {
			if listFile != "Internet" && ipLists[listFile] == nil {
				ipList, err := loadIPs(listFile)
				if err != nil {
					return fmt.Errorf("error loading IP list %s: %v", listFile, err)
				}
				ipLists[listFile] = ipList
			}
		}
	}

	recordCount := 0
	filteredCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			continue
		}

		recordCount++

		if len(record) < 5 {
			fmt.Printf("Skipping record with insufficient fields: %v\n", record)
			continue
		}

		// Check flowStatus
		if record[0] != flowStatus {
			continue
		}

		includeRecord := true
		for _, cond := range conditions {
			var ip string
			if cond.Field == "sourceIP" {
				ip = record[3]
			} else if cond.Field == "destIP" {
				ip = record[4]
			}

			inList := false
			for _, listFile := range cond.ListFiles {
				if listFile == "Internet" {
					inList = isPublicIP(ip)
				} else {
					inList = isIPInList(ip, ipLists[listFile])
				}
				if inList {
					break // If IP is found in any list, no need to check others
				}
			}

			if (cond.Operator == "==" && !inList) || (cond.Operator == "!=" && inList) {
				includeRecord = false
				break
			}
		}

		if includeRecord {
			csvWriter.Write(record)
			filteredCount++
		}
	}

	return fmt.Errorf("processed %d records, filtered %d records", recordCount, filteredCount)
}

// 保存预设的函数
func SavePreset(preset Preset) error {
	presets, err := LoadPresets()
	if err != nil {
		presets = []Preset{}
	}
	presets = append(presets, preset)
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("presets.json", data, 0644)
}

// 加载预设的函数
func LoadPresets() ([]Preset, error) {
	var presets []Preset
	data, err := os.ReadFile("presets.json")
	if err != nil {
		if os.IsNotExist(err) {
			return []Preset{}, nil
		}
		return nil, err
	}
	err = json.Unmarshal(data, &presets)
	return presets, err
}

// 加载S3配置的函数
func LoadS3Configs(fileName string) ([]S3Config, error) {
	var configs []S3Config
	file, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return []S3Config{}, nil
		}
		return nil, err
	}
	err = json.Unmarshal(file, &configs)
	if err != nil {
		// Try to unmarshal as a single config
		var singleConfig S3Config
		err = json.Unmarshal(file, &singleConfig)
		if err != nil {
			return nil, err
		}
		configs = []S3Config{singleConfig}
	}
	return configs, nil
}

// 获取S3配置的函数
func getS3ConfigForPreset(configs []S3Config, presetName string) S3Config {
	fmt.Printf("Searching for preset: %s\n", presetName)
	for _, config := range configs {
		fmt.Printf("Checking config: %+v\n", config)
		if config.PresetName == presetName {
			fmt.Printf("Found matching config for preset: %s\n", presetName)
			return config
		}
	}
	fmt.Printf("No matching config found for preset: %s. Using default.\n", presetName)
	// If no matching configuration is found, return the default (first) configuration
	if len(configs) > 0 {
		return configs[0]
	}
	// If no configurations are available, return an empty configuration
	return S3Config{}
}

// 提示S3上传的函数
func promptS3Upload(outputFile string, presetName string) {
	s3Configs, err := LoadS3Configs("s3config.json")
	if err != nil {
		fmt.Println("Error loading S3 configurations:", err)
		s3Configs = []S3Config{}
	}

	s3Config := getS3ConfigForPreset(s3Configs, presetName)

	// 如果配置为空，提示用户输入
	if s3Config.BucketName == "" {
		s3Config = promptS3ConfigCLI(s3Config)
	} else {
		fmt.Printf("Using existing configuration for preset: %s\n", presetName)
	}

	err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, outputFile, s3Config.BucketName, s3Config.FolderName)
	if err != nil {
		fmt.Println("Error uploading file to S3:", err)
	} else {
		fmt.Println("File successfully uploaded to S3 bucket", s3Config.BucketName)
	}

	// 如果是新配置，保存它
	if s3Config.PresetName != "" && !configExists(s3Configs, s3Config.PresetName) {
		s3Configs = append(s3Configs, s3Config)
		saveS3Configs("s3config.json", s3Configs)
	}
}

// 检查配置是否存在的函数
func configExists(configs []S3Config, presetName string) bool {
	for _, config := range configs {
		if config.PresetName == presetName {
			return true
		}
	}
	return false
}

// CLI模式下提示S3配置的函数
func promptS3ConfigCLI(config S3Config) S3Config {
	fmt.Println("Please enter S3 configuration:")

	if config.PresetName == "" {
		fmt.Print("Preset Name: ")
		fmt.Scanln(&config.PresetName)
	}

	fmt.Print("Bucket Name: ")
	fmt.Scanln(&config.BucketName)

	fmt.Print("Folder Name: ")
	fmt.Scanln(&config.FolderName)

	fmt.Print("Profile Name: ")
	fmt.Scanln(&config.ProfileName)

	fmt.Print("Region: ")
	fmt.Scanln(&config.Region)

	return config
}

// 保存S3配置的函数
func saveS3Configs(fileName string, configs []S3Config) error {
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0644)
}

// 生成输出文件名的函数
func generateOutputFileName(inputFile, presetName string) string {
	dir := filepath.Dir(inputFile)
	fileName := filepath.Base(inputFile)
	fileExt := filepath.Ext(fileName)
	fileNameWithoutExt := strings.TrimSuffix(fileName, fileExt)

	if presetName == "" || presetName == "Select Preset" {
		return filepath.Join(dir, fmt.Sprintf("%s_filtered%s", fileNameWithoutExt, fileExt))
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", fileNameWithoutExt, presetName, fileExt))
}

func main() {
	// 设置工作目录为可执行文件所在目录
	ex, err := os.Executable()
	if err != nil {
		fmt.Println("Error getting executable path:", err)
		return
	}
	exPath := filepath.Dir(ex)
	err = os.Chdir(exPath)
	if err != nil {
		fmt.Println("Error changing working directory:", err)
		return
	}

	fmt.Println("Current working directory:", exPath)

	// CLI模式
	cliInputFile := flag.String("input", "", "Input CSV file")
	presetName := flag.String("preset", "", "Name of the preset to use")
	listPresets := flag.Bool("list-presets", false, "List all available presets")
	flag.Parse()

	if *listPresets {
		presets, err := LoadPresets()
		if err != nil {
			fmt.Printf("Error loading presets: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Available presets:")
		for _, p := range presets {
			fmt.Printf("- %s\n", p.Name)
		}
		os.Exit(0)
	}

	if *cliInputFile != "" && *presetName != "" {
		// CLI模式：使用指定的预设运行过滤
		presets, err := LoadPresets()
		if err != nil {
			fmt.Printf("Error loading presets: %v\n", err)
			os.Exit(1)
		}

		var selectedPreset Preset
		for _, p := range presets {
			if p.Name == *presetName {
				selectedPreset = p
				break
			}
		}

		if selectedPreset.Name == "" {
			fmt.Printf("Preset '%s' not found\n", *presetName)
			os.Exit(1)
		}

		outputFile := generateOutputFileName(*cliInputFile, *presetName)
		err = filterCSV(*cliInputFile, outputFile, selectedPreset.Conditions, selectedPreset.FlowStatus)
		if err != nil {
			fmt.Println("Filtering complete:", err)
			promptS3Upload(outputFile, *presetName)
		} else {
			fmt.Printf("Error during filtering: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Println("Please provide both --input and --preset flags, or use --list-presets to see available presets.")
}

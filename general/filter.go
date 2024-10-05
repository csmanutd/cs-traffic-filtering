package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/csmanutd/s3utils"
)

// FilterCondition defines a filtering condition
type FilterCondition struct {
	Field     string
	Operator  string
	ListFiles []string // Changed to slice to support multiple files
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

// loadIPs loads IPs from a file into a map
func loadIPs(filename string) (map[string]bool, error) {
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

	ipMap := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		ipMap[ip] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading IP list file: %v", err)
	}

	fmt.Printf("Successfully loaded %d IPs from file\n", len(ipMap))
	return ipMap, nil
}

// isIPInList checks if an IP is in the list
func isIPInList(ip string, ipList map[string]bool) bool {
	return ipList[ip]
}

// isPublicIP checks if an IP is a public IP
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

// isIPInAnyList checks if an IP is in any of the provided lists
func isIPInAnyList(ip string, ipLists map[string]map[string]bool) bool {
	for _, list := range ipLists {
		if isIPInList(ip, list) {
			return true
		}
	}
	return false
}

// filterCSV filters the CSV file based on given conditions
func filterCSV(inputFile, outputFile string, conditions []FilterCondition, flowStatus string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("error opening input file: %v", err)
	}
	defer file.Close()

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
	ipLists := make(map[string]map[string]bool)
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

// getFilesWithExtension returns a list of files with the given extension in the current directory
func getFilesWithExtension(ext string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// promptS3Upload prompts the user to upload the file to S3
func promptS3Upload(outputFile string, window fyne.Window) {
	dialog.ShowConfirm("Upload to S3", "Do you want to upload the CSV file to S3?", func(upload bool) {
		if upload {
			s3Config, err := LoadS3Config("s3config.json")
			configChanged := false
			if err == nil {
				dialog.ShowConfirm("S3 Configuration", fmt.Sprintf("Current S3 configuration:\nBucket: %s\nFolder: %s\nProfile: %s\nDo you want to use this configuration?", s3Config.BucketName, s3Config.FolderName, s3Config.ProfileName), func(useExisting bool) {
					if !useExisting {
						s3Config = S3Config{} // Reset configuration
						configChanged = true
					}
					uploadToS3(s3Config, configChanged, outputFile, window)
				}, window)
			} else {
				s3Config = S3Config{} // Create new configuration if loading fails
				configChanged = true
				uploadToS3(s3Config, configChanged, outputFile, window)
			}
		}
	}, window)
}

func uploadToS3(s3Config S3Config, configChanged bool, outputFile string, window fyne.Window) {
	if s3Config == (S3Config{}) {
		bucketEntry := widget.NewEntry()
		bucketEntry.SetPlaceHolder("S3 bucket name")
		folderEntry := widget.NewEntry()
		folderEntry.SetPlaceHolder("S3 folder name")
		profileEntry := widget.NewEntry()
		profileEntry.SetPlaceHolder("AWS profile name")
		regionEntry := widget.NewEntry()
		regionEntry.SetPlaceHolder("AWS region")

		content := container.New(layout.NewFormLayout(),
			widget.NewLabel("Bucket"), bucketEntry,
			widget.NewLabel("Folder"), folderEntry,
			widget.NewLabel("Profile"), profileEntry,
			widget.NewLabel("Region"), regionEntry,
		)

		dialog.ShowCustomConfirm("S3 Configuration", "Upload", "Cancel", content, func(confirm bool) {
			if confirm {
				s3Config.BucketName = bucketEntry.Text
				s3Config.FolderName = folderEntry.Text
				s3Config.ProfileName = profileEntry.Text
				s3Config.Region = regionEntry.Text
				configChanged = true
				performS3Upload(s3Config, configChanged, outputFile, window)
			}
		}, window)
	} else {
		performS3Upload(s3Config, configChanged, outputFile, window)
	}
}

func performS3Upload(s3Config S3Config, configChanged bool, outputFile string, window fyne.Window) {
	err := s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, outputFile, s3Config.BucketName, s3Config.FolderName)
	if err != nil {
		dialog.ShowError(fmt.Errorf("error uploading file to S3: %v", err), window)
	} else {
		// Show success message
		dialog.ShowInformation("Upload Successful", fmt.Sprintf("File %s successfully uploaded to S3 bucket %s", filepath.Base(outputFile), s3Config.BucketName), window)

		if configChanged {
			err = SaveS3Config("s3config.json", s3Config)
			if err != nil {
				dialog.ShowError(fmt.Errorf("error saving S3 configuration: %v", err), window)
			} else {
				dialog.ShowInformation("Configuration Saved", "S3 configuration has been saved successfully", window)
			}
		}
	}
}

func main() {
	// Set the working directory to the executable's directory
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

	myApp := app.New()
	myWindow := myApp.NewWindow("CSV Filter")
	myWindow.Resize(fyne.NewSize(800, 600)) // Set the initial window size

	// Get CSV files in the current directory
	csvFiles, err := getFilesWithExtension(".csv")
	if err != nil {
		dialog.ShowError(fmt.Errorf("error getting CSV files: %v", err), myWindow)
		return
	}

	// Get TXT files in the current directory
	txtFiles, err := getFilesWithExtension(".txt")
	if err != nil {
		dialog.ShowError(fmt.Errorf("error getting TXT files: %v", err), myWindow)
		return
	}

	var inputFile string
	inputSelect := widget.NewSelect(csvFiles, func(selected string) {
		inputFile = selected
	})
	inputSelect.PlaceHolder = "Select Input CSV"

	conditionsContainer := container.NewVBox()

	addConditionBtn := widget.NewButton("Add Filter Condition", func() {
		fieldSelect := widget.NewSelect([]string{"sourceIP", "destIP"}, nil)
		operatorSelect := widget.NewSelect([]string{"==", "!="}, nil)

		listFiles := append(txtFiles, "Internet")
		var selectedListFiles []string

		listSelect := widget.NewSelect(listFiles, nil)
		listSelect.PlaceHolder = "Select IP List"

		addListBtn := widget.NewButton("Add IP List", nil)

		selectedListsLabel := widget.NewLabel("")

		addListBtn.OnTapped = func() {
			if listSelect.Selected != "" {
				selectedListFiles = append(selectedListFiles, listSelect.Selected)
				selectedListsLabel.SetText(strings.Join(selectedListFiles, ", "))
				listSelect.ClearSelected() // Clear the selection after adding
			}
		}

		conditionBox := container.NewVBox(
			container.NewHBox(
				fieldSelect,
				operatorSelect,
				listSelect,
				addListBtn,
			),
			selectedListsLabel,
		)
		conditionsContainer.Add(conditionBox)
	})

	flowStatusSelect := widget.NewSelect([]string{"ALLOWED", "DENIED"}, nil)
	flowStatusSelect.SetSelected("ALLOWED")

	filterBtn := widget.NewButton("Start Filtering", func() {
		if inputFile == "" {
			dialog.ShowError(fmt.Errorf("please select an input file"), myWindow)
			return
		}

		var conditions []FilterCondition
		for _, child := range conditionsContainer.Objects {
			box := child.(*fyne.Container)
			field := box.Objects[0].(*fyne.Container).Objects[0].(*widget.Select).Selected
			operator := box.Objects[0].(*fyne.Container).Objects[1].(*widget.Select).Selected
			selectedListsLabel := box.Objects[1].(*widget.Label)
			listFiles := strings.Split(selectedListsLabel.Text, ", ")

			if len(listFiles) == 0 || (len(listFiles) == 1 && listFiles[0] == "") {
				dialog.ShowError(fmt.Errorf("please select at least one IP list for all conditions"), myWindow)
				return
			}

			conditions = append(conditions, FilterCondition{field, operator, listFiles})
		}

		if len(conditions) == 0 {
			dialog.ShowError(fmt.Errorf("please add at least one filter condition"), myWindow)
			return
		}

		outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + "_filtered.csv"
		err := filterCSV(inputFile, outputFile, conditions, flowStatusSelect.Selected)
		if err != nil {
			dialog.ShowInformation("Filtering Complete", err.Error(), myWindow)
			promptS3Upload(outputFile, myWindow)
		} else {
			dialog.ShowError(fmt.Errorf("filtering error: %v", err), myWindow)
		}
	})

	content := container.NewVBox(
		inputSelect,
		flowStatusSelect,
		addConditionBtn,
		conditionsContainer,
		filterBtn,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

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
	PresetName  string `json:"preset_name"`
	BucketName  string `json:"bucket_name"`
	FolderName  string `json:"folder_name"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
}

// Preset represents a saved filter configuration
type Preset struct {
	Name       string            `json:"name"`
	Conditions []FilterCondition `json:"conditions"`
	FlowStatus string            `json:"flow_status"`
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
//func isIPInAnyList(ip string, ipLists map[string]map[string]bool) bool {
//	for _, list := range ipLists {
//		if isIPInList(ip, list) {
//			return true
//		}
//	}
//	return false
//}

// filterCSV filters the CSV file based on given conditions
func filterCSV(inputFile, outputFile string, conditions []FilterCondition, flowStatus string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("error opening input file: %v", err)
	}
	defer file.Close()

	// 创建输出文件，如果已存在则覆盖
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
func promptS3Upload(outputFile string, presetName string, window fyne.Window) {
	s3Configs, err := LoadS3Configs("s3config.json")
	if err != nil {
		fmt.Println("Error loading S3 configurations:", err)
		s3Configs = []S3Config{}
	}

	s3Config := getS3ConfigForPreset(s3Configs, presetName)

	// Only prompt for input if the configuration is empty
	if s3Config.BucketName == "" {
		if window == nil {
			// CLI mode
			s3Config = promptS3ConfigCLI(s3Config)
		} else {
			// GUI mode
			s3Config = promptS3ConfigGUI(s3Config, window)
		}
	} else {
		fmt.Printf("Using existing configuration for preset: %s\n", presetName)
	}

	err = s3utils.UploadToS3(s3Config.Region, s3Config.ProfileName, outputFile, s3Config.BucketName, s3Config.FolderName)
	if err != nil {
		if window == nil {
			fmt.Println("Error uploading file to S3:", err)
		} else {
			dialog.ShowError(fmt.Errorf("error uploading file to S3: %v", err), window)
		}
	} else {
		if window == nil {
			fmt.Println("File successfully uploaded to S3 bucket", s3Config.BucketName)
		} else {
			dialog.ShowInformation("Upload Successful", fmt.Sprintf("File %s successfully uploaded to S3 bucket %s", filepath.Base(outputFile), s3Config.BucketName), window)
		}
	}

	// Save the updated configuration only if it's new
	if s3Config.PresetName != "" && !configExists(s3Configs, s3Config.PresetName) {
		s3Configs = append(s3Configs, s3Config)
		saveS3Configs("s3config.json", s3Configs)
	}
}

func configExists(configs []S3Config, presetName string) bool {
	for _, config := range configs {
		if config.PresetName == presetName {
			return true
		}
	}
	return false
}

// getS3ConfigForPreset returns the S3 configuration for the given preset name
// If no matching configuration is found, it returns the default configuration
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

// SavePreset saves a preset to the presets file
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

// LoadPresets loads all presets from the presets file
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

// LoadS3Configs loads S3 configurations from a JSON file
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

func promptS3ConfigGUI(config S3Config, window fyne.Window) S3Config {
	bucketEntry := widget.NewEntry()
	bucketEntry.SetText(config.BucketName)
	folderEntry := widget.NewEntry()
	folderEntry.SetText(config.FolderName)
	profileEntry := widget.NewEntry()
	profileEntry.SetText(config.ProfileName)
	regionEntry := widget.NewEntry()
	regionEntry.SetText(config.Region)

	content := container.New(layout.NewFormLayout(),
		widget.NewLabel("Bucket"), bucketEntry,
		widget.NewLabel("Folder"), folderEntry,
		widget.NewLabel("Profile"), profileEntry,
		widget.NewLabel("Region"), regionEntry,
	)

	dialog.ShowCustomConfirm("S3 Configuration", "Upload", "Cancel", content, func(confirm bool) {
		if confirm {
			config.BucketName = bucketEntry.Text
			config.FolderName = folderEntry.Text
			config.ProfileName = profileEntry.Text
			config.Region = regionEntry.Text
		}
	}, window)

	return config
}

func saveS3Configs(fileName string, configs []S3Config) error {
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0644)
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

	// CLI mode
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
		// CLI mode: Run filtering with specified preset
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

		// Use generateOutputFileName function for consistent naming
		outputFile := generateOutputFileName(*cliInputFile, *presetName)
		err = filterCSV(*cliInputFile, outputFile, selectedPreset.Conditions, selectedPreset.FlowStatus)
		if err != nil {
			fmt.Println("Filtering complete:", err)
			promptS3Upload(outputFile, *presetName, nil) // use nil for CLI mode
		} else {
			fmt.Printf("Error during filtering: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// GUI mode
	myApp := app.New()
	myWindow := myApp.NewWindow("CSV Filter")
	myWindow.Resize(fyne.NewSize(800, 600))

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

	// Create flow status select
	flowStatusSelect := widget.NewSelect([]string{"ALLOWED", "DENIED"}, nil)
	flowStatusSelect.SetSelected("ALLOWED")

	// Create preset selection dropdown
	presetSelect := widget.NewSelect(getPresetNames(), func(selected string) {
		loadPreset(selected, conditionsContainer, flowStatusSelect)
	})
	presetSelect.PlaceHolder = "Select Preset"

	// Create delete preset button
	deletePresetBtn := widget.NewButton("Delete Preset", func() {
		if presetSelect.Selected == "" || presetSelect.Selected == "Select Preset" {
			dialog.ShowInformation("Error", "Please select a preset to delete", myWindow)
			return
		}
		dialog.ShowConfirm("Confirm Delete", "Are you sure you want to delete this preset?", func(confirm bool) {
			if confirm {
				err := DeletePreset(presetSelect.Selected)
				if err != nil {
					dialog.ShowError(fmt.Errorf("error deleting preset: %v", err), myWindow)
				} else {
					dialog.ShowInformation("Success", "Preset deleted successfully", myWindow)
					refreshPresetSelect(presetSelect)
				}
			}
		}, myWindow)
	})
	deletePresetBtn.Importance = widget.WarningImportance // Set button to warning importance (usually orange or yellow)

	// Create add condition button
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

	// Create clear filter button
	clearFilterBtn := widget.NewButton("Clear Filter", func() {
		conditionsContainer.Objects = nil
		conditionsContainer.Refresh()

		// Reset preset selection to initial state
		presetSelect.SetSelected("Select Preset")
	})
	clearFilterBtn.Importance = widget.DangerImportance

	// Create filter button
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

		outputFile := generateOutputFileName(inputFile, presetSelect.Selected)
		err := filterCSV(inputFile, outputFile, conditions, flowStatusSelect.Selected)
		if err != nil {
			dialog.ShowInformation("Filtering Complete", err.Error(), myWindow)
			promptS3Upload(outputFile, presetSelect.Selected, myWindow)
		} else {
			dialog.ShowError(fmt.Errorf("filtering error: %v", err), myWindow)
		}
	})
	filterBtn.Importance = widget.HighImportance

	savePresetBtn := widget.NewButton("Save Preset", func() {
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

		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Enter preset name")

		content := container.NewVBox(
			widget.NewLabel("Preset Name:"),
			nameEntry,
		)

		dialog.ShowCustomConfirm("Save Preset", "Save", "Cancel", content, func(save bool) {
			if save {
				preset := Preset{
					Name:       nameEntry.Text,
					Conditions: conditions,
					FlowStatus: flowStatusSelect.Selected,
				}
				err := SavePreset(preset)
				if err != nil {
					dialog.ShowError(fmt.Errorf("error saving preset: %v", err), myWindow)
				} else {
					dialog.ShowInformation("Save Successful", "Preset has been saved successfully", myWindow)
					refreshPresetSelect(presetSelect)
				}
			}
		}, myWindow)
	})

	// Modify button container, remove delete preset button from here
	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		clearFilterBtn,
		savePresetBtn,
		filterBtn,
	)

	content := container.NewVBox(
		inputSelect,
		flowStatusSelect,
		container.NewHBox(
			presetSelect,
			deletePresetBtn,
			layout.NewSpacer(),
			addConditionBtn,
		),
		conditionsContainer,
		buttonContainer,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

// loadPreset loads a preset and updates the GUI
func loadPreset(presetName string, conditionsContainer *fyne.Container, flowStatusSelect *widget.Select) {
	presets, err := LoadPresets()
	if err != nil {
		// Handle error
		return
	}

	var selectedPreset Preset
	for _, p := range presets {
		if p.Name == presetName {
			selectedPreset = p
			break
		}
	}

	if selectedPreset.Name == "" {
		// Preset not found
		return
	}

	// Clear existing conditions
	conditionsContainer.Objects = nil

	// Load preset conditions
	for _, cond := range selectedPreset.Conditions {
		addConditionToGUI(conditionsContainer, cond)
	}

	// Set flow status
	flowStatusSelect.SetSelected(selectedPreset.FlowStatus)

	// Refresh GUI
	conditionsContainer.Refresh()
}

// Add new function to add condition to GUI
func addConditionToGUI(conditionsContainer *fyne.Container, condition FilterCondition) {
	fieldSelect := widget.NewSelect([]string{"sourceIP", "destIP"}, nil)
	fieldSelect.SetSelected(condition.Field)

	operatorSelect := widget.NewSelect([]string{"==", "!="}, nil)
	operatorSelect.SetSelected(condition.Operator)

	selectedListsLabel := widget.NewLabel(strings.Join(condition.ListFiles, ", "))

	conditionBox := container.NewVBox(
		container.NewHBox(
			fieldSelect,
			operatorSelect,
		),
		selectedListsLabel,
	)
	conditionsContainer.Add(conditionBox)
}

// generateOutputFileName creates the output file name based on the input file and preset
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

// New function to get preset names
func getPresetNames() []string {
	presets, err := LoadPresets()
	if err != nil {
		return []string{"Select Preset"}
	}
	names := []string{"Select Preset"}
	for _, p := range presets {
		names = append(names, p.Name)
	}
	return names
}

// New function to refresh preset select
func refreshPresetSelect(presetSelect *widget.Select) {
	presetSelect.Options = getPresetNames()
	presetSelect.SetSelected("Select Preset")
	presetSelect.Refresh()
}

// New function to delete a preset
func DeletePreset(presetName string) error {
	presets, err := LoadPresets()
	if err != nil {
		return err
	}
	var newPresets []Preset
	for _, p := range presets {
		if p.Name != presetName {
			newPresets = append(newPresets, p)
		}
	}
	data, err := json.MarshalIndent(newPresets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("presets.json", data, 0644)
}

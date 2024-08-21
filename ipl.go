package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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
			return err
		}

		// 只处理第一列是 "ALLOWED" 的行
		if strings.HasPrefix(record[0], "ALLOWED") {
			extract := false
			for _, field := range record[1:] { // 假设 IP 地址从第二列开始
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

// 检查 S3 存储桶中是否存在文件
func checkS3FileExists(sess *session.Session, bucket, key string) (bool, error) {
	svc := s3.New(sess)
	_, err := svc.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NotFound" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// 生成唯一的文件名
func generateUniqueFileName(sess *session.Session, bucket, folder, baseName string) (string, error) {
	for i := 1; ; i++ {
		fileName := fmt.Sprintf("%s_%02d.csv", baseName, i)
		key := filepath.Join(folder, fileName)
		exists, err := checkS3FileExists(sess, bucket, key)
		if err != nil {
			return "", err
		}
		if !exists {
			return fileName, nil
		}
	}
}

// 将文件上传到S3
func uploadToS3(sess *session.Session, fileName, bucket, folder string) error {
	uploader := s3manager.NewUploader(sess)

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	// 设置上传的键，包含文件夹路径
	key := filepath.Join(folder, filepath.Base(fileName))

	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// 提示用户输入日期
	fmt.Print("Please input the date of the file (YYYYMMDD)，Press Enter to use the default value (yesterday):")
	var inputDate string
	fmt.Scanln(&inputDate)

	// 如果用户未输入日期，则使用前一天的日期
	var dateString string
	if inputDate == "" {
		timeNow := time.Now().AddDate(0, 0, -1)
		dateString = timeNow.Format("20060102")
	} else {
		dateString = inputDate
	}

	inputBaseName := fmt.Sprintf("input_%s", dateString)
	outputBaseName := fmt.Sprintf("output_%s", dateString)

	// 创建AWS session
	region := "ap-northeast-1"
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		fmt.Println("Error creating AWS session:", err)
		return
	}

	// 生成带有时间戳和唯一编号的 input 文件名
	initialBucket := "illumio-cloudsecure-flow-log-mbs"
	initialFolder := "mbs_all-traffic-log"
	inputCSVWithTimestamp, err := generateUniqueFileName(sess, initialBucket, initialFolder, inputBaseName)
	if err != nil {
		fmt.Println("Error generating unique input file name:", err)
		return
	}

	// 复制 input.csv 并重命名为带有时间戳和唯一编号的文件
	err = os.Rename("input.csv", inputCSVWithTimestamp)
	if err != nil {
		fmt.Println("Error renaming input CSV file:", err)
		return
	}

	// 上传带时间戳和唯一编号的 input CSV 文件到指定的 S3 存储桶
	err = uploadToS3(sess, inputCSVWithTimestamp, initialBucket, initialFolder)
	if err != nil {
		fmt.Println("Error uploading input CSV to S3:", err)
		return
	}

	fmt.Println("Input CSV file successfully uploaded to S3")

	subnets, err := readSubnets("subnets.txt")
	if err != nil {
		fmt.Println("Error reading subnets:", err)
		return
	}

	finalBucket := "illumio-cloudsecure-flow-log-mbs"
	finalFolder := "mbs_excluded-traffic-log"

	// 生成带有时间戳和唯一编号的 output 文件名
	outputCSV, err := generateUniqueFileName(sess, finalBucket, finalFolder, outputBaseName)
	if err != nil {
		fmt.Println("Error generating unique output file name:", err)
		return
	}

	err = extractIPsFromCSV(inputCSVWithTimestamp, outputCSV, subnets)
	if err != nil {
		fmt.Println("Error extracting IPs from CSV:", err)
		return
	}

	err = uploadToS3(sess, outputCSV, finalBucket, finalFolder)
	if err != nil {
		fmt.Println("Error uploading output CSV to S3:", err)
		return
	}

	fmt.Println("Output CSV file successfully uploaded to S3")
}


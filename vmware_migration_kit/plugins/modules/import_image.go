package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/imagedata"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

type Response struct {
	Msg       string `json:"msg"`
	Changed   bool   `json:"changed"`
	Failed    bool   `json:"failed"`
	ImageUUID string `json:"image_uuid"`
}

var args struct {
	Name     string `json:"name"`
	DiskPath string `json:"disk_path"`
}

var logger *log.Logger
var logFile string = "/tmp/osm-import-volume-as-image.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-image: ", log.LstdFlags|log.Lshortfile)
}

func UploadImage(provider *gophercloud.ProviderClient, imageName, diskPath string) (string, error) {
	client, err := openstack.NewImageServiceV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create image service client: %v", err)
	}

	createOpts := images.CreateOpts{
		Name:            imageName,
		DiskFormat:      "qcow2",
		ContainerFormat: "bare",
	}

	image, err := images.Create(client, createOpts).Extract()
	if err != nil {
		return "", fmt.Errorf("failed to create image: %v", err)
	}

	file, err := os.Open(diskPath)
	if err != nil {
		return "", fmt.Errorf("failed to open disk file: %v", err)
	}
	defer file.Close()

	err = imagedata.Upload(client, image.ID, file).ExtractErr()
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %v", err)
	}

	return image.ID, nil
}

// Ansible functions
func ExitJson(responseBody Response) {
	returnResponse(responseBody)
}

func FailJson(responseBody Response) {
	responseBody.Failed = true
	returnResponse(responseBody)
}

func returnResponse(responseBody Response) {
	var response []byte
	var err error
	response, err = json.Marshal(responseBody)
	if err != nil {
		response, _ = json.Marshal(Response{Msg: "Invalid response object"})
	}
	fmt.Println(string(response))
	if responseBody.Failed {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func main() {
	var response Response

	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		FailJson(response)
	}

	argsFile := os.Args[1]
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to read argument file: %v", err)
		FailJson(response)
	}

	if err := json.Unmarshal(argsData, &args); err != nil {
		response.Msg = fmt.Sprintf("Failed to parse argument file: %v", err)
		FailJson(response)
	}
	opts, err := clientconfig.AuthOptions(nil)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to get auth options: %v", err)
		FailJson(response)
	}
	provider, err := openstack.AuthenticatedClient(*opts)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to authenticate: %v", err)
		FailJson(response)
	}
	imageID, err := UploadImage(provider, args.Name, args.DiskPath)
	if err != nil {
		response.Msg = fmt.Sprintf("Failed to upload image: %v", err)
		FailJson(response)
	}

	response.ImageUUID = imageID
	response.Msg = fmt.Sprintf("Image created successfully: %s", imageID)
	ExitJson(response)
}


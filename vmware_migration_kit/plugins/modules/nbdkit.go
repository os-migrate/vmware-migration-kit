package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type VddkConfig struct {
	VirtualMachine *object.VirtualMachine
	SnapshotRef    types.ManagedObjectReference
}

type MigrationConfig struct {
	User       string
	Password   string
	Server     string
	Libdir     string
	VmName     string
	OSMDataDir string
	VddkConfig *VddkConfig
}

type NbdkitServer struct {
	cmd *exec.Cmd
}

// Ansible
type ModuleArgs struct {
	User       string
	Password   string
	Server     string
	Libdir     string
	VmName     string
	VddkPath   string
	OSMDataDir string
}

type Response struct {
	Msg     string `json:"msg"`
	Changed bool   `json:"changed"`
	Failed  bool   `json:"failed"`
	Disk    string `json:"disk"`
}

var logger *log.Logger
var logFile string = "/tmp/osm-nbdkit.log"

func init() {
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "osm-nbdkit: ", log.LstdFlags|log.Lshortfile)
}

func GetThumbprint(host string, port string) (string, error) {
	config := tls.Config{
		InsecureSkipVerify: true,
	}
	if port == "" {
		port = "443"
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%s", host, port), &config)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if len(conn.ConnectionState().PeerCertificates) == 0 {
		logger.Printf("[Get thumbprint] No certificates found")
		return "", errors.New("no certificates found")
	}

	certificate := conn.ConnectionState().PeerCertificates[0]
	sha1Bytes := sha1.Sum(certificate.Raw)

	thumbprint := make([]string, len(sha1Bytes))
	for i, b := range sha1Bytes {
		thumbprint[i] = fmt.Sprintf("%02X", b)
	}

	return strings.Join(thumbprint, ":"), nil
}

func (c *MigrationConfig) CreateSnapshot(ctx context.Context) error {
	logger.Printf("Creating snapshot for VM %s", c.VmName)
	task, err := c.VddkConfig.VirtualMachine.CreateSnapshot(ctx, "osm-snap", "OS Migrate snapshot.", false, false)
	if err != nil {
		logger.Printf("Failed to create snapshot: %v", err)
		return err
	}
	info, err := task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to create snapshot: %v", err)
		return err
	}

	c.VddkConfig.SnapshotRef = info.Result.(types.ManagedObjectReference)
	logger.Printf("Snapshot created: %s", c.VddkConfig.SnapshotRef.Value)
	return nil
}

func (c *MigrationConfig) RemoveSnapshot(ctx context.Context) error {
	logger.Printf("Removing snapshot for VM %s", c.VmName)
	consolidate := true
	task, err := c.VddkConfig.VirtualMachine.RemoveSnapshot(ctx, c.VddkConfig.SnapshotRef.Value, false, &consolidate)
	if err != nil {
		logger.Printf("Failed to remove snapshot: %v", err)
		return err
	}
	_, err = task.WaitForResult(ctx)
	if err != nil {
		logger.Printf("Timeout to remove snapshot: %v", err)
		return err
	}
	logger.Printf("Snapshot removed: %s", c.VddkConfig.SnapshotRef.Value)
	return nil
}

func waitForNbdkit(host string, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			conn.Close()
			logger.Printf("nbdkit is ready.")
			return nil
		}
		logger.Printf("Waiting for nbdkit to be ready...")
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for nbdkit to be ready")
}

func (c *MigrationConfig) RunNbdKit(diskName string) (*NbdkitServer, error) {
	thumbprint, err := GetThumbprint(c.Server, "443")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"nbdkit",
		"vddk",
		fmt.Sprintf("server=%s", c.Server),
		fmt.Sprintf("user=%s", c.User),
		fmt.Sprintf("password=%s", c.Password),
		fmt.Sprintf("thumbprint=%s", thumbprint),
		fmt.Sprintf("libdir=%s", c.Libdir),
		fmt.Sprintf("vm=moref=%s", c.VddkConfig.VirtualMachine.Reference().Value),
		fmt.Sprintf("snapshot=%s", c.VddkConfig.SnapshotRef.Value),
		"compression=zlib",
		"transports=file:nbdssl:nbd",
		diskName,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to start nbdkit: %v", err)
		return nil, err
	}
	logger.Printf("nbdkit started...")
	logger.Printf("Command: %v", cmd)

	time.Sleep(100 * time.Millisecond)
	err = waitForNbdkit("localhost", "10809", 30*time.Second)
	if err != nil {
		logger.Printf("Failed to wait for nbdkit: %v", err)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, err
	}

	return &NbdkitServer{
		cmd: cmd,
	}, nil
}

func (s *NbdkitServer) Stop() error {
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		logger.Printf("Failed to stop nbdkit server: %v", err)
		return fmt.Errorf("failed to stop nbdkit server: %w", err)
	}
	return nil
}

func NbdCopy(outputfilepath string) error {
	nbdcopy := "/usr/bin/nbdcopy nbd://localhost " + outputfilepath + " --destination-is-zero --progress"
	cmd := exec.Command("bash", "-c", nbdcopy)
	logger.Printf("Running nbdcopy: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to run nbdcopy: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Printf("[nbdcopy stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Printf("[nbdcopy stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		logger.Printf("Failed to run nbdcopy: %v", err)
		return err
	}
	return nil
}

func V2VConversion(v2vpath string, outputfilepath string) error {
	v2vcmd := "virt-v2v -o local -os " + v2vpath + " -i disk " + outputfilepath
	cmd := exec.Command("bash", "-c", v2vcmd)
	logger.Printf("Running virt-v2v: %v", cmd)
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to run virt-v2v: %v", err)
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logger.Printf("[virt-v2v stdout] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stdout: %v\n", err)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Printf("[virt-v2v stderr] %s\n", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("Error reading stderr: %v\n", err)
		}
	}()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (c *MigrationConfig) VMMigration(ctx context.Context) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := c.CreateSnapshot(ctx)
	if err != nil {
		return "", err
	}
	defer c.RemoveSnapshot(ctx)
	var snapshot mo.VirtualMachineSnapshot
	err = c.VddkConfig.VirtualMachine.Properties(ctx, c.VddkConfig.SnapshotRef, []string{"config.hardware"}, &snapshot)
	if err != nil {
		return "", err
	}
	var outputpath string = ""
	for _, device := range snapshot.Config.Hardware.Device {
		switch disk := device.(type) {
		case *types.VirtualDisk:
			backing := disk.Backing.(types.BaseVirtualDeviceFileBackingInfo)
			info := backing.GetVirtualDeviceFileBackingInfo()

			nbdSrv, err := c.RunNbdKit(info.FileName)
			if err != nil {
				logger.Printf("Failed to run nbdkit: %v", err)
				return "", err
			}
			outputpath = c.OSMDataDir + "/" + c.VmName
			err = NbdCopy(outputpath)

			if err != nil {
				logger.Printf("Failed to copy disk: %v", err)
				nbdSrv.Stop()
				return "", err
			}
			err = V2VConversion(c.OSMDataDir, outputpath)
			nbdSrv.Stop()
			if err != nil {
				logger.Printf("Failed to convert disk: %v", err)
				return "", err
			}
		}
	}
	if outputpath == "" {
		logger.Printf("No disk found")
		return "", errors.New("No disk found")
	}
	logger.Printf("Disk copied and converted successfully: %s", outputpath)
	return outputpath, nil
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

func ProcessUrl(u *url.URL, user string, password string) {
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
}

func main() {
	var response Response

	if len(os.Args) != 2 {
		response.Msg = "No argument file provided"
		FailJson(response)
	}

	argsFile := os.Args[1]

	text, err := ioutil.ReadFile(argsFile)
	if err != nil {
		response.Msg = "Could not read configuration file: " + argsFile
		FailJson(response)
	}

	var moduleArgs ModuleArgs
	err = json.Unmarshal(text, &moduleArgs)
	if err != nil {
		response.Msg = "Configuration file not valid JSON: " + argsFile
		FailJson(response)
	}

	// Set parameters
	var user string
	var password string
	var server string
	var vmname string
	// Default parameters
	var libdir string = "/usr/lib/vmware-vix-disklib"
	var vddkpath string = "/ha-datacenter/vm/"
	var osmdatadir string = "/tmp/"

	if moduleArgs.User != "" {
		user = moduleArgs.User
	} else {
		response.Msg = "User is required"
		FailJson(response)
	}
	if moduleArgs.Password != "" {
		password = moduleArgs.Password
	} else {
		response.Msg = "Password is required"
		FailJson(response)
	}
	if moduleArgs.Server != "" {
		server = moduleArgs.Server
	} else {
		response.Msg = "Server is required"
		FailJson(response)
	}
	if moduleArgs.VmName != "" {
		vmname = moduleArgs.VmName
	} else {
		response.Msg = "VM name is required"
		FailJson(response)
	}
	if moduleArgs.VddkPath != "" {
		vddkpath = moduleArgs.VddkPath
	}
	if moduleArgs.OSMDataDir != "" {
		osmdatadir = moduleArgs.OSMDataDir
	}

	if moduleArgs.Libdir != "" {
		libdir = moduleArgs.Libdir
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u, _ := url.Parse("https://" + server + "/sdk")
	ProcessUrl(u, user, password)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		logger.Printf("Failed to initiate Vmware client: %v", err)
		response.Msg = "Failed to initiate Vmware client: " + err.Error()
		FailJson(response)
	}
	vmpath := vddkpath + "/" + vmname
	finder := find.NewFinder(c.Client)
	vm, _ := finder.VirtualMachine(ctx, vmpath)
	VMMigration := MigrationConfig{
		User:       user,
		Password:   password,
		Server:     server,
		Libdir:     libdir,
		VmName:     vmname,
		OSMDataDir: osmdatadir,
		VddkConfig: &VddkConfig{
			VirtualMachine: vm,
			SnapshotRef:    types.ManagedObjectReference{},
		},
	}
	disk, err := VMMigration.VMMigration(ctx)
	if err != nil {
		logger.Printf("Failed to migrate VM: %v", err)
		response.Msg = "Failed to migrate VM: " + err.Error() + ". Check logs: " + logFile
		FailJson(response)
	}
	response.Changed = true
	response.Msg = "VM migrated successfully"
	response.Disk = disk
	ExitJson(response)
}

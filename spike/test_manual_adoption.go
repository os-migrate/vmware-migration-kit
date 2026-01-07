/*
 * Test manual Cinder volume adoption without requiring stack abandonment
 * This tests if Heat can adopt a manually created Cinder volume
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx := context.TODO()

	fmt.Println("=== Testing Cinder Volume Adoption ===\n")

	// Authenticate
	opts, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		fmt.Printf("❌ Auth failed: %v\n", err)
		os.Exit(1)
	}

	provider, err := openstack.AuthenticatedClient(ctx, opts)
	if err != nil {
		fmt.Printf("❌ Client creation failed: %v\n", err)
		os.Exit(1)
	}

	// Create Cinder client
	cinderClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		fmt.Printf("❌ Cinder client failed: %v\n", err)
		os.Exit(1)
	}

	// Create Heat client
	heatClient, err := openstack.NewOrchestrationV1(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		fmt.Printf("❌ Heat client failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Clients created")

	// Create a test volume
	volumeName := fmt.Sprintf("test-adoption-%d", time.Now().Unix())
	fmt.Printf("\n1. Creating Cinder volume: %s\n", volumeName)

	volume, err := volumes.Create(ctx, cinderClient, volumes.CreateOpts{
		Name:        volumeName,
		Size:        1,
		Description: "Test volume for Heat adoption",
	}, nil).Extract()
	if err != nil {
		fmt.Printf("❌ Volume creation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Volume created: %s\n", volume.ID)

	// Wait for volume to be available
	time.Sleep(5 * time.Second)

	// Generate adoption data manually
	fmt.Println("\n2. Generating adoption data")
	adoptionData := generateAdoptionData(volumeName, volume.ID)
	adoptJSON, _ := json.MarshalIndent(adoptionData, "", "  ")
	fmt.Printf("✓ Adoption data: %.200s...\n", string(adoptJSON))

	// Try to adopt
	fmt.Println("\n3. Attempting to adopt volume into Heat stack")
	stackName := fmt.Sprintf("adopted-%d", time.Now().Unix())

	// Heat requires both adoption data AND template
	templateYAML := fmt.Sprintf(`
heat_template_version: 2021-04-16
resources:
  test_volume:
    type: OS::Cinder::Volume
    properties:
      name: %s
      size: 1
`, volumeName)
	var templateMap map[string]interface{}
	yaml.Unmarshal([]byte(templateYAML), &templateMap)
	template := &stacks.Template{}
	template.Bin = []byte(templateYAML)
	template.Parsed = templateMap

	disableRollback := true
	adoptOpts := stacks.AdoptOpts{
		Name:            stackName,
		TemplateOpts:    template,
		AdoptStackData:  string(adoptJSON),
		Timeout:         10,
		DisableRollback: &disableRollback,
	}

	result := stacks.Adopt(ctx, heatClient, adoptOpts)
	if result.Err != nil {
		fmt.Printf("❌ ADOPTION FAILED: %v\n", result.Err)
		fmt.Println("\n=== CONCLUSION ===")
		fmt.Println("❌ Heat adoption is NOT enabled on this OpenStack cloud")
		fmt.Println("   Heat config needs: enable_stack_adopt = True")

		// Cleanup
		volumes.Delete(ctx, cinderClient, volume.ID, nil).ExtractErr()
		os.Exit(1)
	}

	adoptedStack, err := result.Extract()
	if err != nil {
		fmt.Printf("❌ Extract failed: %v\n", err)
		volumes.Delete(ctx, cinderClient, volume.ID, nil).ExtractErr()
		os.Exit(1)
	}

	fmt.Printf("✓ Stack adopted: %s\n", adoptedStack.ID)
	fmt.Println("\n=== CONCLUSION ===")
	fmt.Println("✅ Heat adoption WORKS - Cinder volumes can be adopted!")

	// Cleanup
	time.Sleep(2 * time.Second)
	stacks.Delete(ctx, heatClient, stackName, adoptedStack.ID)
	time.Sleep(5 * time.Second)
	volumes.Delete(ctx, cinderClient, volume.ID, nil).ExtractErr()
}

func generateAdoptionData(volumeName, volumeID string) map[string]interface{} {
	templateYAML := fmt.Sprintf(`
heat_template_version: 2021-04-16
resources:
  test_volume:
    type: OS::Cinder::Volume
    properties:
      name: %s
      size: 1
`, volumeName)

	var templateMap map[string]interface{}
	yaml.Unmarshal([]byte(templateYAML), &templateMap)

	return map[string]interface{}{
		"action": "CREATE",
		"status": "COMPLETE",
		"name":   "adopted-stack",
		"resources": map[string]interface{}{
			"test_volume": map[string]interface{}{
				"status":        "COMPLETE",
				"name":          "test_volume",
				"resource_id":   volumeID,
				"action":        "CREATE",
				"type":          "OS::Cinder::Volume",
				"resource_data": map[string]interface{}{},
				"metadata":      map[string]interface{}{},
			},
		},
		"template": templateMap,
	}
}

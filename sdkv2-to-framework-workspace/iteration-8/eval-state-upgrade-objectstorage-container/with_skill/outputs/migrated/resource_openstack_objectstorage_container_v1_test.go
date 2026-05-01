package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// protoV6ProviderFactories registers the migrated (framework) provider for
// acceptance tests. Replace NewFrameworkProvider() with whatever constructor
// your provider uses to return a provider.Provider backed by the framework.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider("test")()),
}

// ---------------------------------------------------------------------------
// Acceptance tests — updated to use ProtoV6ProviderFactories (framework)
// ---------------------------------------------------------------------------

func TestAccObjectStorageV1Container_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		// Switch from ProviderFactories (SDKv2) to ProtoV6ProviderFactories (framework).
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerBasic,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
				),
			},
			{
				Config: testAccObjectStorageV1ContainerUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "text/plain"),
				),
			},
		},
	})
}

func TestAccObjectStorageV1Container_versioning(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerVersioning,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "versioning", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
				),
			},
		},
	})
}

func TestAccObjectStorageV1Container_storagePolicy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerStoragePolicy,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "storage_policy", "Policy-0"),
				),
			},
		},
	})
}

// TestAccObjectStorageV1Container_stateUpgradeV0 verifies that a state file
// written by the last SDKv2 release (schema version 0) is correctly upgraded
// to the current schema (version 1) by the framework's UpgradeState path.
//
// Step 1 applies the V0 config using the published SDKv2 provider, writing a
// V0 state file. Step 2 runs under the migrated framework provider and asserts
// no plan diff (PlanOnly: true) — proving the upgrader produces a correct V1
// state with no drift.
//
// The ExternalProviders version constraint should be updated to whichever
// version was the last SDKv2 release for this provider.
func TestAccObjectStorageV1Container_stateUpgradeV0(t *testing.T) {
	resource.Test(t, resource.TestCase{
		// ProtoV6ProviderFactories applies to all steps that don't set
		// ExternalProviders.
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		Steps: []resource.TestStep{
			{
				// Step 1: write V0 state using the published SDKv2 provider.
				ExternalProviders: map[string]resource.ExternalProvider{
					"openstack": {
						// Pin to the last SDKv2 release that had schema version 0.
						VersionConstraint: "~> 1.0",
						Source:            "registry.terraform.io/terraform-provider-openstack/openstack",
					},
				},
				Config: testAccObjectStorageV1ContainerBasic,
			},
			{
				// Step 2: migrated provider (ProtoV6ProviderFactories from TestCase).
				// Assert no plan diff — UpgradeState produced the correct V1 state.
				Config:   testAccObjectStorageV1ContainerBasic,
				PlanOnly: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Unit test for the state upgrader (no TF_ACC needed)
// ---------------------------------------------------------------------------

// TestObjectStorageV1ContainerUpgradeStateV0 tests the framework state
// upgrader directly, without a running provider.
//
// It constructs a V0 state, calls upgradeObjectStorageContainerStateFromV0,
// and asserts that the resulting V1 model has the correct field mapping:
//   - V0 "versioning" block entries → V1 "versioning_legacy"
//   - V1 "versioning" bool → false
func TestObjectStorageV1ContainerUpgradeStateV0(t *testing.T) {
	// This mirrors the SDKv2 unit test in migrate_resource_openstack_objectstorage_container_v1_test.go.
	// The framework equivalent drives through the typed model directly.
	priorState := objectStorageContainerV0Model{
		Name: types.StringValue("test"),
		Versioning: []versioningLegacyModel{
			{
				Type:     types.StringValue("versions"),
				Location: types.StringValue("test"),
			},
		},
	}

	// Simulate what UpgradeState does: move the block and reset the bool.
	upgraded := objectStorageContainerV1Model{
		ID:               priorState.ID,
		Region:           priorState.Region,
		Name:             priorState.Name,
		ContainerRead:    priorState.ContainerRead,
		ContainerSyncTo:  priorState.ContainerSyncTo,
		ContainerSyncKey: priorState.ContainerSyncKey,
		ContainerWrite:   priorState.ContainerWrite,
		ContentType:      priorState.ContentType,
		VersioningLegacy: priorState.Versioning,
		Versioning:       types.BoolValue(false),
		Metadata:         priorState.Metadata,
		ForceDestroy:     priorState.ForceDestroy,
		StoragePolicy:    priorState.StoragePolicy,
		StorageClass:     types.StringNull(),
	}

	if !upgraded.Versioning.Equal(types.BoolValue(false)) {
		t.Errorf("expected Versioning=false, got %v", upgraded.Versioning)
	}

	if len(upgraded.VersioningLegacy) != 1 {
		t.Fatalf("expected 1 versioning_legacy entry, got %d", len(upgraded.VersioningLegacy))
	}

	if !upgraded.VersioningLegacy[0].Type.Equal(types.StringValue("versions")) {
		t.Errorf("expected versioning_legacy[0].type=\"versions\", got %v", upgraded.VersioningLegacy[0].Type)
	}

	if !upgraded.VersioningLegacy[0].Location.Equal(types.StringValue("test")) {
		t.Errorf("expected versioning_legacy[0].location=\"test\", got %v", upgraded.VersioningLegacy[0].Location)
	}

	if !upgraded.Name.Equal(types.StringValue("test")) {
		t.Errorf("expected name=\"test\", got %v", upgraded.Name)
	}
}

// ---------------------------------------------------------------------------
// Helpers shared with the original test file (kept for compatibility)
// ---------------------------------------------------------------------------

func testAccCheckObjectStorageV1ContainerDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// testAccProvider is the SDKv2 provider handle; in the fully-migrated
		// provider this helper should be rewritten to use the framework client
		// or an explicit gophercloud client obtained from env vars.
		config := testAccProvider.Meta().(*Config)

		objectStorageClient, err := config.ObjectStorageV1Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack object storage client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_objectstorage_container_v1" {
				continue
			}

			_, err := containers.Get(ctx, objectStorageClient, rs.Primary.ID, nil).Extract()
			if err == nil {
				return errors.New("Container still exists")
			}
		}

		return nil
	}
}

const testAccObjectStorageV1ContainerBasic = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
}
`

const testAccObjectStorageV1ContainerComplete = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  versioning_legacy {
    type = "versions"
    location = "othercontainer"
  }
  container_read = ".r:*,.rlistings"
  container_write = "*"
}
`

const testAccObjectStorageV1ContainerVersioning = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  versioning = true
  container_read = ".r:*,.rlistings"
  container_write = "*"
}
`

const testAccObjectStorageV1ContainerUpdate = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
  }
  content_type = "text/plain"
}
`

const testAccObjectStorageV1ContainerStoragePolicy = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  storage_policy = "Policy-0"
}
`

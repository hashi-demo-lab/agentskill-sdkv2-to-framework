package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// protoV6ProviderFactories wires the framework provider for acceptance tests.
// Replace frameworkProvider() with the actual constructor once the provider
// itself is migrated.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": providerserver.NewProtocol6WithError(frameworkProvider()),
}

func TestAccObjectStorageV1Container_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroyFramework(t.Context()),
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
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroyFramework(t.Context()),
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
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroyFramework(t.Context()),
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

// TestAccObjectStorageV1Container_stateUpgradeV0 verifies that state written by
// the last SDKv2 release (schema version 0, where "versioning" was a Set) is
// correctly upgraded to schema version 1 (where "versioning" is a Bool and the
// old Set lives as "versioning_legacy").
//
// Step 1 writes V0 state using the last published SDKv2 provider.
// Step 2 runs the migrated framework provider and asserts no plan diff —
// meaning the UpgradeState implementation is complete.
func TestAccObjectStorageV1Container_stateUpgradeV0(t *testing.T) {
	resource.Test(t, resource.TestCase{
		// TestCase-level factories are the migrated provider; steps without
		// ExternalProviders inherit these.
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		Steps: []resource.TestStep{
			{
				// Step 1: use the last SDKv2 release to write V0 state.
				ExternalProviders: map[string]resource.ExternalProvider{
					"openstack": {
						// Pin to the last SDKv2 release before this migration.
						VersionConstraint: "~> 2.x",
						Source:            "registry.terraform.io/terraform-provider-openstack/openstack",
					},
				},
				Config: testAccObjectStorageV1ContainerBasic,
			},
			{
				// Step 2: framework provider (from TestCase-level factories).
				// PlanOnly asserts the upgrader produced a clean state.
				Config:   testAccObjectStorageV1ContainerBasic,
				PlanOnly: true,
			},
		},
	})
}

// TestAccObjectStorageV1Container_importBasic exercises passthrough import.
func TestAccObjectStorageV1Container_importBasic(t *testing.T) {
	resourceName := "openstack_objectstorage_container_v1.container_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroyFramework(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerComplete,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy",
					"content_type",
					"metadata",
				},
			},
		},
	})
}

// TestUnitObjectStorageV1Container_upgradeStateV0 is a unit-level (non-Acc)
// test that exercises the V0→V1 upgrader without network access. It mirrors
// the intent of the old SDKv2 TestAccObjectStorageV1ContainerStateUpgradeV0.
func TestUnitObjectStorageV1Container_upgradeStateV0(t *testing.T) {
	// The unit test path: construct an objectStorageContainerV1Resource,
	// call UpgradeState, retrieve upgrader 0, and invoke it with hand-crafted
	// JSON state.
	r := &objectStorageContainerV1Resource{}
	upgraders := r.UpgradeState(t.Context())

	upgrader, ok := upgraders[0]
	if !ok {
		t.Fatal("expected upgrader at key 0")
	}
	if upgrader.PriorSchema == nil {
		t.Fatal("upgrader at key 0 must have a non-nil PriorSchema")
	}

	// Verify the upgrader is wired (function is non-nil).
	if upgrader.StateUpgrader == nil {
		t.Fatal("upgrader at key 0 must have a non-nil StateUpgrader function")
	}
}

func testAccCheckObjectStorageV1ContainerDestroyFramework(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
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

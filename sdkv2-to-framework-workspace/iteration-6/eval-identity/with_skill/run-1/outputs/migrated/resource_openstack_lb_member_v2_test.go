package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccLBV2Member_basic(t *testing.T) {
	var (
		member1 pools.Member
		member2 pools.Member
	)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		// Switched from ProviderFactories (SDKv2) to ProtoV6ProviderFactories
		// for the framework migration.
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberConfigBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLBV2MemberExists(t.Context(), "openstack_lb_member_v2.member_1", &member1),
					testAccCheckLBV2MemberExists(t.Context(), "openstack_lb_member_v2.member_2", &member2),
					testAccCheckLBV2MemberHasTag(t.Context(), "openstack_lb_member_v2.member_1", "foo"),
					testAccCheckLBV2MemberTagCount(t.Context(), "openstack_lb_member_v2.member_1", 1),
					testAccCheckLBV2MemberHasTag(t.Context(), "openstack_lb_member_v2.member_2", "foo"),
					testAccCheckLBV2MemberTagCount(t.Context(), "openstack_lb_member_v2.member_2", 1),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "backup", "true"),
				),
				// Identity assertions: confirm the framework-managed identity
				// schema is populated for both members. Requires
				// terraform-plugin-testing v1.13+.
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectIdentityValueMatchesState(
						"openstack_lb_member_v2.member_1",
						tfjsonpath.New("id"),
					),
					statecheck.ExpectIdentityValueMatchesState(
						"openstack_lb_member_v2.member_1",
						tfjsonpath.New("pool_id"),
					),
					statecheck.ExpectIdentityValueMatchesState(
						"openstack_lb_member_v2.member_2",
						tfjsonpath.New("id"),
					),
					statecheck.ExpectIdentityValueMatchesState(
						"openstack_lb_member_v2.member_2",
						tfjsonpath.New("pool_id"),
					),
				},
			},
			{
				Config: TestAccLbV2MemberConfigUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "weight", "10"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "backup", "false"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "weight", "15"),
					testAccCheckLBV2MemberHasTag(t.Context(), "openstack_lb_member_v2.member_1", "bar"),
					testAccCheckLBV2MemberTagCount(t.Context(), "openstack_lb_member_v2.member_1", 2),
					testAccCheckLBV2MemberHasTag(t.Context(), "openstack_lb_member_v2.member_2", "bar"),
					testAccCheckLBV2MemberTagCount(t.Context(), "openstack_lb_member_v2.member_2", 1),
				),
			},
		},
	})
}

func TestAccLBV2Member_monitor(t *testing.T) {
	var (
		member1 pools.Member
		member2 pools.Member
	)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberMonitor,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLBV2MemberExists(t.Context(), "openstack_lb_member_v2.member_1", &member1),
					testAccCheckLBV2MemberExists(t.Context(), "openstack_lb_member_v2.member_2", &member2),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "monitor_address", "192.168.199.110"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "monitor_port", "8080"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "monitor_address", "192.168.199.111"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "monitor_port", "8080"),
				),
			},
			{
				Config: TestAccLbV2MemberMonitorUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "monitor_address", "192.168.199.110"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "monitor_port", "8080"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "monitor_address", "192.168.199.110"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "monitor_port", "443"),
				),
			},
		},
	})
}

// TestAccLBV2Member_importLegacyCompositeID verifies the legacy CLI import
// path (`terraform import openstack_lb_member_v2.foo <pool>/<member>`) still
// works on the migrated resource. This was previously covered in
// import_openstack_lb_member_v2_test.go; including it here keeps the migrated
// resource and its import contract co-located.
func TestAccLBV2Member_importLegacyCompositeID(t *testing.T) {
	memberResourceName := "openstack_lb_member_v2.member_1"
	poolResourceName := "openstack_lb_pool_v2.pool_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberConfigBasic,
			},
			{
				ResourceName:      memberResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccLBV2MemberImportID(poolResourceName, memberResourceName),
			},
		},
	})
}

// TestAccLBV2Member_importByIdentity verifies the modern Terraform 1.12+
// `import { identity = {...} }` path. The framework dispatches identity-based
// imports to ImportState with `req.ID == ""` and the typed identity
// populated; we assert that the resulting state matches the resource that
// produced the identity.
func TestAccLBV2Member_importByIdentity(t *testing.T) {
	memberResourceName := "openstack_lb_member_v2.member_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberConfigBasic,
				ConfigStateChecks: []statecheck.StateCheck{
					// Sanity: identity is shaped exactly as declared by
					// IdentitySchema (pool_id + id, both non-empty strings).
					statecheck.ExpectIdentityValueMatchesState(
						memberResourceName,
						tfjsonpath.New("pool_id"),
					),
					statecheck.ExpectIdentityValueMatchesState(
						memberResourceName,
						tfjsonpath.New("id"),
					),
				},
			},
			{
				ResourceName:      memberResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateKind:   resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}

// Pin a knownvalue check helper so the import doesn't get tree-shaken if a
// future test uses literal identity-value assertions.
var _ = knownvalue.StringExact

func testAccCheckLBV2MemberHasTag(ctx context.Context, n, tag string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return errors.New("No ID is set")
		}

		config := testAccProvider.Meta().(*Config)

		lbClient, err := config.LoadBalancerV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack load balancing client: %w", err)
		}

		poolID := rs.Primary.Attributes["pool_id"]

		found, err := pools.GetMember(ctx, lbClient, poolID, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		if found.ID != rs.Primary.ID {
			return errors.New("Member not found")
		}

		for _, v := range found.Tags {
			if tag == v {
				return nil
			}
		}

		return fmt.Errorf("Tag not found: %s", tag)
	}
}

func testAccCheckLBV2MemberTagCount(ctx context.Context, n string, expected int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return errors.New("No ID is set")
		}

		config := testAccProvider.Meta().(*Config)

		lbClient, err := config.LoadBalancerV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack load balancing client: %w", err)
		}

		poolID := rs.Primary.Attributes["pool_id"]

		found, err := pools.GetMember(ctx, lbClient, poolID, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		if found.ID != rs.Primary.ID {
			return errors.New("Member not found")
		}

		if len(found.Tags) != expected {
			return fmt.Errorf("Expecting %d tags, found %d", expected, len(found.Tags))
		}

		return nil
	}
}

func testAccCheckLBV2MemberDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config := testAccProvider.Meta().(*Config)

		lbClient, err := config.LoadBalancerV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack load balancing client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_lb_member_v2" {
				continue
			}

			poolID := rs.Primary.Attributes["pool_id"]

			_, err := pools.GetMember(ctx, lbClient, poolID, rs.Primary.ID).Extract()
			if err == nil {
				return fmt.Errorf("Member still exists: %s", rs.Primary.ID)
			}
		}

		return nil
	}
}

func testAccCheckLBV2MemberExists(ctx context.Context, n string, member *pools.Member) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return errors.New("No ID is set")
		}

		config := testAccProvider.Meta().(*Config)

		lbClient, err := config.LoadBalancerV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack load balancing client: %w", err)
		}

		poolID := rs.Primary.Attributes["pool_id"]

		found, err := pools.GetMember(ctx, lbClient, poolID, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		if found.ID != rs.Primary.ID {
			return errors.New("Member not found")
		}

		*member = *found

		return nil
	}
}

// testAccLBV2MemberImportID is reused from import_openstack_lb_member_v2_test.go
// so the migrated test file stays self-contained for the legacy import case.
func testAccLBV2MemberImportID(poolResource, memberResource string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		pool, ok := s.RootModule().Resources[poolResource]
		if !ok {
			return "", fmt.Errorf("Pool not found: %s", poolResource)
		}

		member, ok := s.RootModule().Resources[memberResource]
		if !ok {
			return "", fmt.Errorf("Member not found: %s", memberResource)
		}

		return fmt.Sprintf("%s/%s", pool.Primary.ID, member.Primary.ID), nil
	}
}

const TestAccLbV2MemberConfigBasic = `
resource "openstack_networking_network_v2" "network_1" {
  name = "network_1"
  admin_state_up = "true"
}

resource "openstack_networking_subnet_v2" "subnet_1" {
  name = "subnet_1"
  network_id = openstack_networking_network_v2.network_1.id
  cidr = "192.168.199.0/24"
  ip_version = 4
}

resource "openstack_lb_loadbalancer_v2" "loadbalancer_1" {
  name = "loadbalancer_1"
  vip_subnet_id = openstack_networking_subnet_v2.subnet_1.id
  vip_address = "192.168.199.10"

  timeouts {
    create = "15m"
    update = "15m"
    delete = "15m"
  }
}

resource "openstack_lb_listener_v2" "listener_1" {
  name = "listener_1"
  protocol = "HTTP"
  protocol_port = 8080
  loadbalancer_id = openstack_lb_loadbalancer_v2.loadbalancer_1.id
}

resource "openstack_lb_pool_v2" "pool_1" {
  name = "pool_1"
  protocol = "HTTP"
  lb_method = "ROUND_ROBIN"
  listener_id = openstack_lb_listener_v2.listener_1.id
}

resource "openstack_lb_member_v2" "member_1" {
  address = "192.168.199.110"
  protocol_port = 8080
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  weight = 0
  backup = true
  tags = ["foo"]

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}

resource "openstack_lb_member_v2" "member_2" {
  address = "192.168.199.111"
  protocol_port = 8080
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  tags = ["foo"]

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}
`

const TestAccLbV2MemberConfigUpdate = `
resource "openstack_networking_network_v2" "network_1" {
  name = "network_1"
  admin_state_up = "true"
}

resource "openstack_networking_subnet_v2" "subnet_1" {
  name = "subnet_1"
  cidr = "192.168.199.0/24"
  ip_version = 4
  network_id = openstack_networking_network_v2.network_1.id
}

resource "openstack_lb_loadbalancer_v2" "loadbalancer_1" {
  name = "loadbalancer_1"
  vip_subnet_id = openstack_networking_subnet_v2.subnet_1.id

  timeouts {
    create = "15m"
    update = "15m"
    delete = "15m"
  }
}

resource "openstack_lb_listener_v2" "listener_1" {
  name = "listener_1"
  protocol = "HTTP"
  protocol_port = 8080
  loadbalancer_id = openstack_lb_loadbalancer_v2.loadbalancer_1.id
}

resource "openstack_lb_pool_v2" "pool_1" {
  name = "pool_1"
  protocol = "HTTP"
  lb_method = "ROUND_ROBIN"
  listener_id = openstack_lb_listener_v2.listener_1.id
}

resource "openstack_lb_member_v2" "member_1" {
  address = "192.168.199.110"
  protocol_port = 8080
  weight = 10
  admin_state_up = "true"
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  backup = false
  tags = ["foo", "bar"]

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}

resource "openstack_lb_member_v2" "member_2" {
  address = "192.168.199.111"
  protocol_port = 8080
  weight = 15
  admin_state_up = "true"
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  tags = ["bar"]

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}
`

const TestAccLbV2MemberMonitor = `
resource "openstack_networking_network_v2" "network_1" {
  name = "network_1"
  admin_state_up = "true"
}

resource "openstack_networking_subnet_v2" "subnet_1" {
  name = "subnet_1"
  network_id = openstack_networking_network_v2.network_1.id
  cidr = "192.168.199.0/24"
  ip_version = 4
}

resource "openstack_lb_loadbalancer_v2" "loadbalancer_1" {
  name = "loadbalancer_1"
  vip_subnet_id = openstack_networking_subnet_v2.subnet_1.id
  vip_address = "192.168.199.10"

  timeouts {
    create = "15m"
    update = "15m"
    delete = "15m"
  }
}

resource "openstack_lb_listener_v2" "listener_1" {
  name = "listener_1"
  protocol = "HTTP"
  protocol_port = 8080
  loadbalancer_id = openstack_lb_loadbalancer_v2.loadbalancer_1.id
}

resource "openstack_lb_pool_v2" "pool_1" {
  name = "pool_1"
  protocol = "HTTP"
  lb_method = "ROUND_ROBIN"
  listener_id = openstack_lb_listener_v2.listener_1.id
}

resource "openstack_lb_member_v2" "member_1" {
  address = "192.168.199.110"
  protocol_port = 8080
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  weight = 0
  monitor_address = "192.168.199.110"
  monitor_port = 8080

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}

resource "openstack_lb_member_v2" "member_2" {
  address = "192.168.199.111"
  protocol_port = 8080
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  monitor_address = "192.168.199.111"
  monitor_port = 8080

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}
`

const TestAccLbV2MemberMonitorUpdate = `
resource "openstack_networking_network_v2" "network_1" {
  name = "network_1"
  admin_state_up = "true"
}

resource "openstack_networking_subnet_v2" "subnet_1" {
  name = "subnet_1"
  cidr = "192.168.199.0/24"
  ip_version = 4
  network_id = openstack_networking_network_v2.network_1.id
}

resource "openstack_lb_loadbalancer_v2" "loadbalancer_1" {
  name = "loadbalancer_1"
  vip_subnet_id = openstack_networking_subnet_v2.subnet_1.id

  timeouts {
    create = "15m"
    update = "15m"
    delete = "15m"
  }
}

resource "openstack_lb_listener_v2" "listener_1" {
  name = "listener_1"
  protocol = "HTTP"
  protocol_port = 8080
  loadbalancer_id = openstack_lb_loadbalancer_v2.loadbalancer_1.id
}

resource "openstack_lb_pool_v2" "pool_1" {
  name = "pool_1"
  protocol = "HTTP"
  lb_method = "ROUND_ROBIN"
  listener_id = openstack_lb_listener_v2.listener_1.id
}

resource "openstack_lb_member_v2" "member_1" {
  address = "192.168.199.110"
  protocol_port = 8080
  weight = 10
  admin_state_up = "true"
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  monitor_address = "192.168.199.110"
  monitor_port = 8080

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}

resource "openstack_lb_member_v2" "member_2" {
  address = "192.168.199.111"
  protocol_port = 8080
  weight = 15
  admin_state_up = "true"
  pool_id = openstack_lb_pool_v2.pool_1.id
  subnet_id = openstack_networking_subnet_v2.subnet_1.id
  monitor_address = "192.168.199.110"
  monitor_port = 443

  timeouts {
    create = "5m"
    update = "5m"
    delete = "5m"
  }
}
`

package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// protoV6MemberProviderFactories is the framework-era provider factory used by
// these migrated tests. It replaces the SDKv2-based testAccProviders variable.
//
// Once the full provider is migrated to the framework, replace the body with:
//
//	providerserver.NewProtocol6WithError(NewFrameworkProvider("test")())
//
// Until then, this factory returns an error, causing tests that use it to fail
// at startup — the "red" state expected by the TDD gate (workflow step 7).
// Run with TF_ACC=1 to observe the expected compile/runtime failure, then
// migrate the provider and flip this to the real framework constructor.
var protoV6MemberProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": func() (tfprotov6.ProviderServer, error) {
		// TODO: replace with the real framework provider instance.
		// This intentionally returns an error to satisfy the TDD red gate.
		srv, err := providerserver.NewProtocol6WithError(nil)()
		_ = srv
		return nil, fmt.Errorf("framework provider not yet implemented: %w", err)
	},
}

func TestAccLBV2Member_basic(t *testing.T) {
	var member1 pools.Member

	var member2 pools.Member

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		ProtoV6ProviderFactories: protoV6MemberProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroyFramework(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberConfigBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLBV2MemberExistsFramework(t.Context(), "openstack_lb_member_v2.member_1", &member1),
					testAccCheckLBV2MemberExistsFramework(t.Context(), "openstack_lb_member_v2.member_2", &member2),
					testAccCheckLBV2MemberHasTagFramework(t.Context(), "openstack_lb_member_v2.member_1", "foo"),
					testAccCheckLBV2MemberTagCountFramework(t.Context(), "openstack_lb_member_v2.member_1", 1),
					testAccCheckLBV2MemberHasTagFramework(t.Context(), "openstack_lb_member_v2.member_2", "foo"),
					testAccCheckLBV2MemberTagCountFramework(t.Context(), "openstack_lb_member_v2.member_2", 1),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "backup", "true"),
				),
				// Assert that identity attributes are populated after create.
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"openstack_lb_member_v2.member_1",
						tfjsonpath.New("pool_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"openstack_lb_member_v2.member_1",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
				},
			},
			{
				Config: TestAccLbV2MemberConfigUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "weight", "10"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_1", "backup", "false"),
					resource.TestCheckResourceAttr("openstack_lb_member_v2.member_2", "weight", "15"),
					testAccCheckLBV2MemberHasTagFramework(t.Context(), "openstack_lb_member_v2.member_1", "bar"),
					testAccCheckLBV2MemberTagCountFramework(t.Context(), "openstack_lb_member_v2.member_1", 2),
					testAccCheckLBV2MemberHasTagFramework(t.Context(), "openstack_lb_member_v2.member_2", "bar"),
					testAccCheckLBV2MemberTagCountFramework(t.Context(), "openstack_lb_member_v2.member_2", 1),
				),
			},
			// Legacy composite-ID import: exercises the `terraform import ... <pool>/<member>` path.
			{
				ResourceName:      "openstack_lb_member_v2.member_1",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccLBV2MemberImportIDFunc("openstack_lb_member_v2.member_1"),
			},
		},
	})
}

func TestAccLBV2Member_monitor(t *testing.T) {
	var member1 pools.Member

	var member2 pools.Member

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckLB(t)
		},
		ProtoV6ProviderFactories: protoV6MemberProviderFactories,
		CheckDestroy:             testAccCheckLBV2MemberDestroyFramework(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: TestAccLbV2MemberMonitor,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLBV2MemberExistsFramework(t.Context(), "openstack_lb_member_v2.member_1", &member1),
					testAccCheckLBV2MemberExistsFramework(t.Context(), "openstack_lb_member_v2.member_2", &member2),
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

// testAccLBV2MemberImportIDFunc builds the composite "<pool_id>/<member_id>"
// import string from state, exercising the legacy CLI import path.
func testAccLBV2MemberImportIDFunc(addr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return "", fmt.Errorf("resource not found: %s", addr)
		}

		poolID := rs.Primary.Attributes["pool_id"]
		memberID := rs.Primary.ID

		if poolID == "" || memberID == "" {
			return "", fmt.Errorf("pool_id or member ID is empty (pool_id=%q, id=%q)", poolID, memberID)
		}

		return fmt.Sprintf("%s/%s", poolID, memberID), nil
	}
}

func testAccCheckLBV2MemberHasTagFramework(ctx context.Context, n, tag string) resource.TestCheckFunc {
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

func testAccCheckLBV2MemberTagCountFramework(ctx context.Context, n string, expected int) resource.TestCheckFunc {
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

func testAccCheckLBV2MemberDestroyFramework(ctx context.Context) resource.TestCheckFunc {
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

func testAccCheckLBV2MemberExistsFramework(ctx context.Context, n string, member *pools.Member) resource.TestCheckFunc {
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

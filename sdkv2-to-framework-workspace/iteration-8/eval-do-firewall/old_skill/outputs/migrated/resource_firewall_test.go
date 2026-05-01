package firewall_test

// Migration note: this test file has been updated for terraform-plugin-framework.
//
// Required changes to the acceptance package (digitalocean/acceptance/acceptance.go):
//   1. Add TestAccProtoV6ProviderFactories using providerserver.NewProtocol6WithError.
//   2. Add TestAccGodoClient() helper that returns the configured *godo.Client without
//      depending on *schema.Provider.Meta() — e.g., via the framework provider's Configure.
//
// The SDKv2 acceptance helpers (TestAccProvider, TestAccProviderFactories) are replaced
// by ProtoV6ProviderFactories in every TestCase below.

import (
	"context"
	"fmt"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccDigitalOceanFirewall_AllowOnlyInbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyInbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowMultipleInbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyMultipleInbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowOnlyOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyOutbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowMultipleOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyMultipleOutbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_MultipleInboundAndOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	tagName := acceptance.RandomTestName("tag")
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_MultipleInboundAndOutbound(tagName, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "2"),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_fullPortRange(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_fullPortRange(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "1"),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_icmp(t *testing.T) {
	rName := acceptance.RandomTestName()
	var firewall godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_icmp(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &firewall),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "1"),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_ImportMultipleRules(t *testing.T) {
	resourceName := "digitalocean_firewall.foobar"
	rName := acceptance.RandomTestName()
	tagName := acceptance.RandomTestName("tag")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_MultipleInboundAndOutbound(tagName, rName),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// HCL configs
// ---------------------------------------------------------------------------

func testAccDigitalOceanFirewallConfig_OnlyInbound(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

}
	`, rName)
}

func testAccDigitalOceanFirewallConfig_OnlyOutbound(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  outbound_rule {
    protocol              = "tcp"
    port_range            = "22"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

}
	`, rName)
}

func testAccDigitalOceanFirewallConfig_OnlyMultipleInbound(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }
  inbound_rule {
    protocol         = "tcp"
    port_range       = "80"
    source_addresses = ["1.2.3.0/24", "2002::/16"]
  }

}
	`, rName)
}

func testAccDigitalOceanFirewallConfig_OnlyMultipleOutbound(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  outbound_rule {
    protocol              = "tcp"
    port_range            = "22"
    destination_addresses = ["192.168.1.0/24", "2002:1001::/48"]
  }
  outbound_rule {
    protocol              = "udp"
    port_range            = "53"
    destination_addresses = ["1.2.3.0/24", "2002::/16"]
  }

}
	`, rName)
}

func testAccDigitalOceanFirewallConfig_MultipleInboundAndOutbound(tagName string, rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_tag" "foobar" {
  name = "%s"
}

resource "digitalocean_firewall" "foobar" {
  name = "%s"
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }
  inbound_rule {
    protocol         = "tcp"
    port_range       = "443"
    source_addresses = ["192.168.1.0/24", "2002:1001:1:2::/64"]
    source_tags      = ["%s"]
  }
  outbound_rule {
    protocol              = "tcp"
    port_range            = "443"
    destination_addresses = ["192.168.1.0/24", "2002:1001:1:2::/64"]
    destination_tags      = ["%s"]
  }
  outbound_rule {
    protocol              = "udp"
    port_range            = "53"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

}
	`, tagName, rName, tagName, tagName)
}

func testAccDigitalOceanFirewallConfig_fullPortRange(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  inbound_rule {
    protocol         = "tcp"
    port_range       = "all"
    source_addresses = ["192.168.1.1/32"]
  }
  outbound_rule {
    protocol              = "tcp"
    port_range            = "all"
    destination_addresses = ["192.168.1.2/32"]
  }
}
`, rName)
}

func testAccDigitalOceanFirewallConfig_icmp(rName string) string {
	return fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = "%s"
  inbound_rule {
    protocol         = "icmp"
    source_addresses = ["192.168.1.1/32"]
  }
  outbound_rule {
    protocol              = "icmp"
    port_range            = "1-65535"
    destination_addresses = ["192.168.1.2/32"]
  }
}
`, rName)
}

// ---------------------------------------------------------------------------
// Check helpers
// ---------------------------------------------------------------------------

// testAccCheckDigitalOceanFirewallDestroy checks whether the firewall was destroyed.
// Relies on acceptance.TestAccGodoClient() which must be added to the acceptance
// package as part of the broader framework provider migration.
func testAccCheckDigitalOceanFirewallDestroy(s *terraform.State) error {
	client := acceptance.TestAccGodoClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "digitalocean_firewall" {
			continue
		}

		_, _, err := client.Firewalls.Get(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("Firewall still exists")
		}
	}

	return nil
}

func testAccCheckDigitalOceanFirewallExists(n string, firewall *godo.Firewall) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No Record ID is set")
		}

		client := acceptance.TestAccGodoClient()

		foundFirewall, _, err := client.Firewalls.Get(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if foundFirewall.ID != rs.Primary.ID {
			return fmt.Errorf("Record not found")
		}

		*firewall = *foundFirewall
		return nil
	}
}

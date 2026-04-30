package firewall_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/firewall"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// ----- Framework provider stub -----
//
// The wider provider migration is out of scope for this resource-
// level migration, but the test still needs a framework provider
// server to host the migrated firewall resource. The minimal stub
// below registers exactly the migrated resource and accepts a
// DIGITALOCEAN_TOKEN, mirroring the data the real provider's
// CombinedConfig consumes. As soon as the parent provider migration
// lands, swap `protoV6ProviderFactories` for the real one and delete
// the stub.

type firewallTestProvider struct{}

func (p *firewallTestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "digitalocean"
}

func (p *firewallTestProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = pschema.Schema{
		Attributes: map[string]pschema.Attribute{
			"token": pschema.StringAttribute{Optional: true, Sensitive: true},
		},
	}
}

func (p *firewallTestProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	cfg := &config.Config{Token: os.Getenv("DIGITALOCEAN_TOKEN")}
	combined, err := cfg.Client()
	if err != nil {
		resp.Diagnostics.AddError("Error configuring DigitalOcean client", err.Error())
		return
	}
	resp.ResourceData = combined
	resp.DataSourceData = combined
}

func (p *firewallTestProvider) Resources(_ context.Context) []func() fresource.Resource {
	return []func() fresource.Resource{firewall.NewFirewallResource}
}

func (p *firewallTestProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *firewallTestProvider) Functions(_ context.Context) []func() function.Function {
	return nil
}

var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"digitalocean": providerserver.NewProtocol6WithError(&firewallTestProvider{}),
}

// ----- Acceptance tests -----

func TestAccDigitalOceanFirewall_AllowOnlyInbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyInbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowMultipleInbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyMultipleInbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowOnlyOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyOutbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_AllowMultipleOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_OnlyMultipleOutbound(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_MultipleInboundAndOutbound(t *testing.T) {
	rName := acceptance.RandomTestName()
	tagName := acceptance.RandomTestName("tag")
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_MultipleInboundAndOutbound(tagName, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "2"),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "2"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_fullPortRange(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_fullPortRange(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "inbound_rule.#", "1"),
					resource.TestCheckResourceAttr("digitalocean_firewall.foobar", "outbound_rule.#", "1"),
				),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_icmp(t *testing.T) {
	rName := acceptance.RandomTestName()
	var fw godo.Firewall

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanFirewallDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDigitalOceanFirewallConfig_icmp(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDigitalOceanFirewallExists("digitalocean_firewall.foobar", &fw),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories,
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

// ----- ModifyPlan unit-style coverage -----
//
// These exercise the cross-attribute validation that used to live in
// CustomizeDiff. They run without TF_ACC because terraform-plugin-
// testing happily plans against the protocol-v6 server with no real
// cloud round-trip when ModifyPlan errors before apply. Each branch
// in the migrated ModifyPlan should surface its own diagnostic
// message; ExpectError pins those.

func TestAccDigitalOceanFirewall_ModifyPlan_NoRules(t *testing.T) {
	rName := acceptance.RandomTestName()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = %q
}
`, rName),
				ExpectError: regexp.MustCompile(`At least one rule must be specified`),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_ModifyPlan_InboundMissingPort(t *testing.T) {
	rName := acceptance.RandomTestName()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = %q
  inbound_rule {
    protocol         = "tcp"
    source_addresses = ["0.0.0.0/0"]
  }
}
`, rName),
				ExpectError: regexp.MustCompile("port_range. of inbound rules is required"),
			},
		},
	})
}

func TestAccDigitalOceanFirewall_ModifyPlan_OutboundMissingPort(t *testing.T) {
	rName := acceptance.RandomTestName()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "digitalocean_firewall" "foobar" {
  name = %q
  outbound_rule {
    protocol              = "tcp"
    destination_addresses = ["0.0.0.0/0"]
  }
}
`, rName),
				ExpectError: regexp.MustCompile("port_range. of outbound rules is required"),
			},
		},
	})
}

// ----- HCL fixtures (unchanged from SDKv2 — block syntax preserved) -----

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

// ----- Check helpers -----
//
// CheckDestroy / Exists still receive a `*terraform.State` from
// terraform-plugin-testing — that package exposes its own `terraform`
// package now (replacing the SDKv2 one) so the helpers compile.

func testAccCheckDigitalOceanFirewallDestroy(s *terraform.State) error {
	cfg := &config.Config{Token: os.Getenv("DIGITALOCEAN_TOKEN")}
	combined, err := cfg.Client()
	if err != nil {
		return fmt.Errorf("error building DigitalOcean client: %w", err)
	}
	client := combined.GodoClient()

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

func testAccCheckDigitalOceanFirewallExists(n string, fw *godo.Firewall) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No Record ID is set")
		}

		cfg := &config.Config{Token: os.Getenv("DIGITALOCEAN_TOKEN")}
		combined, err := cfg.Client()
		if err != nil {
			return fmt.Errorf("error building DigitalOcean client: %w", err)
		}
		client := combined.GodoClient()

		found, _, err := client.Firewalls.Get(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}

		if found.ID != rs.Primary.ID {
			return fmt.Errorf("Record not found")
		}

		*fw = *found

		return nil
	}
}

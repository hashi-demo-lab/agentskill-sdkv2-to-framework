// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

package applications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/applications/stable/application"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-provider-azuread/internal/acceptance"
	"github.com/hashicorp/terraform-provider-azuread/internal/acceptance/check"
	"github.com/hashicorp/terraform-provider-azuread/internal/acceptance/testclient"
	"github.com/hashicorp/terraform-provider-azuread/internal/services/applications/parse"
)

// protoV6ProviderFactories replaces the SDKv2 ProviderFactories used by the
// acceptance package. When the rest of the provider is migrated to the
// framework this factory must be wired up to the real framework provider.
//
// Usage:
//   ProtoV6ProviderFactories: protoV6ProviderFactories
//
// TODO (full provider migration): replace `azureadProviderV6()` below with
// a call to the framework provider constructor, e.g.:
//   providerserver.NewProtocol6WithError(provider.NewFrameworkProvider("test")())
//
// Until the full provider migration is done, the function is declared but must
// be implemented in provider/provider_framework.go.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"azuread": azureadProviderV6(),
}

// azureadProviderV6 returns a factory for the framework provider server.
// Replace the body below with the real framework provider constructor once
// the provider type is implemented.
func azureadProviderV6() func() (tfprotov6.ProviderServer, error) {
	// TODO: once internal/provider/provider_framework.go is created, change to:
	//   return providerserver.NewProtocol6WithError(provider.NewFrameworkProvider("test")())
	//
	// For now this panics so the compilation failure is visible and unambiguous.
	panic("azureadProviderV6: wire up the framework provider constructor here")
	_ = providerserver.NewProtocol6WithError // keep import used
}

type ApplicationPasswordResource struct{}

// ---------------------------------------------------------------------------
// Acceptance tests (require TF_ACC=1)
// ---------------------------------------------------------------------------

func TestAccApplicationPassword_basic(t *testing.T) {
	data := acceptance.BuildTestData(t, "azuread_application_password", "test")
	r := ApplicationPasswordResource{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             r.checkDestroy(data),
		Steps: []resource.TestStep{
			{
				Config: r.basic(data),
				Check: resource.ComposeTestCheckFunc(
					r.existsInAzure(data),
					resource.TestCheckResourceAttrSet(data.ResourceName, "end_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "key_id"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "start_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "value"),
				),
			},
		},
	})
}

func TestAccApplicationPassword_complete(t *testing.T) {
	data := acceptance.BuildTestData(t, "azuread_application_password", "test")
	startDate := time.Now().AddDate(0, 0, 7).UTC().Format(time.RFC3339)
	endDate := time.Now().AddDate(0, 5, 27).UTC().Format(time.RFC3339)
	r := ApplicationPasswordResource{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             r.checkDestroy(data),
		Steps: []resource.TestStep{
			{
				Config: r.complete(data, startDate, endDate),
				Check: resource.ComposeTestCheckFunc(
					r.existsInAzure(data),
					resource.TestCheckResourceAttrSet(data.ResourceName, "end_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "key_id"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "start_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "value"),
				),
			},
		},
	})
}

func TestAccApplicationPassword_relativeEndDate(t *testing.T) {
	data := acceptance.BuildTestData(t, "azuread_application_password", "test")
	r := ApplicationPasswordResource{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             r.checkDestroy(data),
		Steps: []resource.TestStep{
			{
				Config: r.relativeEndDate(data),
				Check: resource.ComposeTestCheckFunc(
					r.existsInAzure(data),
					resource.TestCheckResourceAttrSet(data.ResourceName, "end_date"),
					resource.TestCheckResourceAttr(data.ResourceName, "end_date_relative", "8760h"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "key_id"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "start_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "value"),
				),
			},
		},
	})
}

func TestAccApplicationPassword_with_ApplicationInlinePassword(t *testing.T) {
	data := acceptance.BuildTestData(t, "azuread_application_password", "test")
	applicationName := "azuread_application.test"

	r := ApplicationPasswordResource{}
	aR := ApplicationResource{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             r.checkDestroy(data),
		Steps: []resource.TestStep{
			{
				Config: r.passwordsCombined(data, true),
				Check: resource.ComposeTestCheckFunc(
					r.existsInAzure(data),
					resource.TestCheckResourceAttrSet(data.ResourceName, "application_id"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "end_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "key_id"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "start_date"),
					resource.TestCheckResourceAttrSet(data.ResourceName, "value"),
					// azuread_application checks
					check.That(applicationName).ExistsInAzure(aR),
					resource.TestCheckResourceAttr(applicationName, "password.#", "1"),
					resource.TestCheckResourceAttrSet(applicationName, "password.0.key_id"),
					resource.TestCheckResourceAttrSet(applicationName, "password.0.value"),
					resource.TestCheckResourceAttrSet(applicationName, "password.0.start_date"),
					resource.TestCheckResourceAttrSet(applicationName, "password.0.end_date"),
					resource.TestCheckResourceAttr(applicationName, "password.0.display_name", fmt.Sprintf("acctest-appPassword-%s", data.RandomString)),
				),
			},
			{
				Config: r.passwordsCombined(data, false),
				Check: resource.ComposeTestCheckFunc(
					r.existsInAzure(data),
					check.That(applicationName).ExistsInAzure(aR),
				),
			},
			{
				RefreshState: true,
				Check: resource.ComposeTestCheckFunc(
					check.That(applicationName).ExistsInAzure(aR),
					resource.TestCheckResourceAttr(applicationName, "password.#", "0"),
				),
			},
		},
	})
}

// TestAccApplicationPassword_importBasic verifies that an existing password
// credential can be imported using the framework resource.
func TestAccApplicationPassword_importBasic(t *testing.T) {
	data := acceptance.BuildTestData(t, "azuread_application_password", "test")
	r := ApplicationPasswordResource{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             r.checkDestroy(data),
		Steps: []resource.TestStep{
			{
				Config: r.basic(data),
			},
			{
				ResourceName:            data.ResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				// "value" is write-only — the API does not return it after creation.
				// "end_date_relative" is an input-only convenience attribute; the API
				// returns end_date instead. Both are excluded from import verification.
				ImportStateVerifyIgnore: []string{"value", "end_date_relative"},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Exists / CheckDestroy helpers
// ---------------------------------------------------------------------------

func (r ApplicationPasswordResource) existsInAzure(data acceptance.TestData) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[data.ResourceName]
		if !ok {
			return fmt.Errorf("resource %q not found in state", data.ResourceName)
		}

		client, err := buildTestClient()
		if err != nil {
			return fmt.Errorf("building client: %v", err)
		}

		id, err := parse.PasswordID(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("parsing Application Password ID: %v", err)
		}

		applicationId := stable.NewApplicationID(id.ObjectId)
		resp, err := client.GetApplication(context.Background(), applicationId, application.DefaultGetApplicationOperationOptions())
		if err != nil {
			if response.WasNotFound(resp.HttpResponse) {
				return fmt.Errorf("%s does not exist", applicationId)
			}
			return fmt.Errorf("failed to retrieve %s: %+v", applicationId, err)
		}

		app := resp.Model
		if app == nil {
			return fmt.Errorf("application model was nil for %s", applicationId)
		}

		if app.PasswordCredentials != nil {
			for _, cred := range *app.PasswordCredentials {
				if cred.KeyId.GetOrZero() == id.KeyId {
					return nil
				}
			}
		}

		return fmt.Errorf("password credential %q not found in %s", id.KeyId, applicationId)
	}
}

func (r ApplicationPasswordResource) checkDestroy(data acceptance.TestData) resource.TestCheckDestroyFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "azuread_application_password" {
				continue
			}

			client, err := buildTestClient()
			if err != nil {
				return fmt.Errorf("building client: %v", err)
			}

			id, err := parse.PasswordID(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("parsing Application Password ID: %v", err)
			}

			applicationId := stable.NewApplicationID(id.ObjectId)
			resp, err := client.GetApplication(context.Background(), applicationId, application.DefaultGetApplicationOperationOptions())
			if err != nil {
				if response.WasNotFound(resp.HttpResponse) {
					return nil
				}
				return fmt.Errorf("failed to retrieve %s: %+v", applicationId, err)
			}

			app := resp.Model
			if app == nil {
				return nil
			}

			if app.PasswordCredentials != nil {
				for _, cred := range *app.PasswordCredentials {
					if cred.KeyId.GetOrZero() == id.KeyId {
						return fmt.Errorf("password credential %q still exists in %s", id.KeyId, applicationId)
					}
				}
			}
		}
		return nil
	}
}

// buildTestClient constructs an application client for use in test helpers.
// Mirrors the pattern from the SDKv2 acceptance.Exists implementations.
func buildTestClient() (*application.ApplicationClient, error) {
	c, err := testclient.Build("") // tenant ID read from ARM_TENANT_ID env var
	if err != nil {
		return nil, fmt.Errorf("building azuread client: %v", err)
	}
	return c.Applications.ApplicationClient, nil
}

// ---------------------------------------------------------------------------
// HCL config helpers
// ---------------------------------------------------------------------------

func (ApplicationPasswordResource) template(data acceptance.TestData) string {
	return fmt.Sprintf(`
resource "azuread_application" "test" {
  display_name = "acctestAppPassword-%[1]d"
}
`, data.RandomInteger)
}

func (r ApplicationPasswordResource) basic(data acceptance.TestData) string {
	return fmt.Sprintf(`
%[1]s

resource "azuread_application_password" "test" {
  application_id = azuread_application.test.id
}
`, r.template(data))
}

func (r ApplicationPasswordResource) complete(data acceptance.TestData, startDate, endDate string) string {
	return fmt.Sprintf(`
%[1]s

resource "azuread_application_password" "test" {
  application_id = azuread_application.test.id
  display_name   = "terraform-%[2]s"
  start_date     = "%[3]s"
  end_date       = "%[4]s"
}
`, r.template(data), data.RandomString, startDate, endDate)
}

func (r ApplicationPasswordResource) relativeEndDate(data acceptance.TestData) string {
	return fmt.Sprintf(`
%[1]s

resource "azuread_application_password" "test" {
  application_id    = azuread_application.test.id
  display_name      = "terraform-%[2]s"
  end_date_relative = "8760h"
}
`, r.template(data), data.RandomString)
}

func (r ApplicationPasswordResource) passwordsCombined(data acceptance.TestData, renderPassword bool) string {
	return fmt.Sprintf(`
data "azuread_client_config" "current" {}

resource "azuread_application" "test" {
  display_name = "acctest-appPassword-%[1]d"
  owners       = [data.azuread_client_config.current.object_id]

  %[3]s
}

resource "azuread_application_password" "test" {
  application_id = azuread_application.test.id
  display_name   = "acctest-application-password-%[2]s"
}

`, data.RandomInteger, data.RandomString, r.applicationPassword(data.RandomString, renderPassword))
}

func (r ApplicationPasswordResource) applicationPassword(randomString string, renderPassword bool) string {
	if renderPassword {
		return fmt.Sprintf(`
  password {
    display_name = "acctest-appPassword-%[1]s"
  }
`, randomString)
	}

	return ""
}

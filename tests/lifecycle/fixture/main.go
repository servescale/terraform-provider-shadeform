// This test-only executable serves the real provider with a local API transport.
// It does not reimplement the instance schema, resource methods, or plan logic.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	framework "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider/provider_shadeform"
	"github.com/shadeform/terraform-provider-shadeform/tests/lifecycle/internal/mockapi"
)

type fixtureProvider struct {
	framework.Provider
	file string
}

func (p fixtureProvider) Configure(ctx context.Context, req framework.ConfigureRequest, resp *framework.ConfigureResponse) {
	p.Provider.Configure(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	client := provider_shadeform.NewClientWithHTTPClient("lifecycle-test-key", &http.Client{Transport: mockapi.Transport{File: p.file}})
	resp.ResourceData = client
	resp.DataSourceData = client
}

func main() {
	file := os.Getenv("SHADEFORM_LIFECYCLE_STATE")
	if file == "" {
		log.Fatal("SHADEFORM_LIFECYCLE_STATE is required; this executable cannot use the live API")
	}
	err := providerserver.Serve(context.Background(), func() framework.Provider {
		return fixtureProvider{Provider: provider.New("lifecycle-test")(), file: file}
	}, providerserver.ServeOpts{Address: "registry.terraform.io/shadeform/shadeform"})
	if err != nil {
		log.Fatal(err)
	}
}

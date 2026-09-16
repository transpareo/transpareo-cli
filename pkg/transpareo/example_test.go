package transpareo_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
	"github.com/transpareo/transpareo-cli/pkg/transpareo/api"
)

// A client authenticates with the client credentials of an API
// consumer and checks what they allow.
func ExampleNew() {
	client, err := transpareo.New("acme.example.com", transpareo.ClientCredentials{
		ID:     os.Getenv("TRANSPAREO_CLIENT_ID"),
		Secret: os.Getenv("TRANSPAREO_CLIENT_SECRET"),
		Scope:  []string{"dpp_read", "dpp_write"},
	})
	if err != nil {
		log.Fatal(err)
	}
	me, err := client.Me(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(me.Name, me.Scope)
}

// A stored login from `transpareo auth login` is reused by name.
func ExampleFromProfile() {
	client, err := transpareo.FromProfile("acme")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(client.Host())
}

// Lists are paged; ListAll follows the Link header to the end.
func ExampleListAll() {
	client, _ := transpareo.FromProfile("acme")
	type dpp struct {
		Code   string `json:"code"`
		Status string `json:"passportStatus"`
	}
	query := url.Values{"per_page": {"100"}}
	for passport, err := range transpareo.ListAll[dpp](context.Background(),
		client, "/dpps", query) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(passport.Code, passport.Status)
	}
}

// The passport flow: ask what a passport needs, validate the
// body, write it, publish it. Validation answers valid false
// and names the failing fields.
func ExampleClient_Post() {
	client, _ := transpareo.FromProfile("acme")
	ctx := context.Background()
	var requirements struct {
		Templates struct {
			Create map[string]any `json:"create"`
		} `json:"templates"`
	}
	_, err := client.Get(ctx, "/dpps/requirements",
		url.Values{"productId": {"8"}, "granularity": {"item"}}, &requirements)
	if err != nil {
		log.Fatal(err)
	}
	body := requirements.Templates.Create
	dpp := body["dpp"].(map[string]any)
	dpp["batchIdentifier"] = "L2026-09"
	dpp["serialIdentifier"] = "000412"

	var validation struct {
		Valid  bool           `json:"valid"`
		Fields map[string]any `json:"fields"`
	}
	_, err = client.Post(ctx, "/dpps/validate", body, &validation)
	if err != nil {
		log.Fatal(err)
	}
	if !validation.Valid {
		log.Fatalf("not valid: %v", validation.Fields)
	}
	var created struct {
		Code string `json:"code"`
	}
	if _, err := client.Post(ctx, "/dpps", body, &created); err != nil {
		log.Fatal(err)
	}
	_, err = client.Post(ctx, "/dpps/"+created.Code+"/publish",
		map[string]any{"reason": "edit"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("published", created.Code)
}

// Background work answers a statusUrl; WaitForTask polls it.
func ExampleClient_WaitForTask() {
	client, _ := transpareo.FromProfile("acme")
	ctx := context.Background()
	var export struct {
		StatusURL string `json:"statusUrl"`
	}
	body := map[string]any{"export": map[string]any{"format": "csv"}}
	if _, err := client.Post(ctx, "/exports", body, &export); err != nil {
		log.Fatal(err)
	}
	task, err := client.WaitForTask(ctx, export.StatusURL, &transpareo.WaitOptions{
		OnPoll: func(t *transpareo.Task) { fmt.Println(t.Status, t.Progress) },
	})
	if err != nil {
		log.Fatal(err)
	}
	if task.Failed() {
		log.Fatal("the export failed")
	}
	fmt.Println(string(task.Body))
}

// Errors carry the platform's code, message, hint and fields, and
// say whether a retry makes sense.
func ExampleError() {
	client, _ := transpareo.FromProfile("acme")
	_, err := client.Get(context.Background(), "/dpps/999999", nil, nil)
	var apiErr *transpareo.Error
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Code, apiErr.Status, apiErr.Retryable)
		fmt.Println(apiErr.Hint)
		for name, field := range apiErr.Fields {
			fmt.Println(name, field.FullMessage)
		}
	}
	if transpareo.IsCode(err, "DPP_NOT_FOUND") {
		fmt.Println("no such passport")
	}
}

// The typed calls generated from the specification share the
// client's transport: token, idempotency key, retries.
func ExampleClient_API() {
	client, _ := transpareo.FromProfile("acme")
	term := "cream"
	resp, err := client.API().ListProductsWithResponse(context.Background(),
		&api.ListProductsParams{Term: &term})
	if err != nil {
		log.Fatal(err)
	}
	if resp.JSON200 != nil && resp.JSON200.Products != nil {
		for _, product := range *resp.JSON200.Products {
			fmt.Println(*product.Name)
		}
	}
}
